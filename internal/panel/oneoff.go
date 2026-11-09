package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// maxTeamRuns is how many one-off runs a team can have going at once, so a busy CI or agent can't fill a small server.
	maxTeamRuns = 3
	// liveOutputLimit is how much output a run keeps in memory for people following it.
	liveOutputLimit = 4 << 20
	keepOneOffRuns  = 100
	triggerAPI      = "api"
)

// liveRun is a one-off run in progress, with its output so far for anyone following it.
type liveRun struct {
	team   string
	mu     sync.Mutex
	out    []byte
	cut    bool
	done   chan struct{}
	cancel context.CancelFunc
}

func (l *liveRun) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.out)+len(b) > liveOutputLimit {
		if !l.cut {
			l.out = append(l.out, "\n[output past 4 MB isn't kept]\n"...)
			l.cut = true
		}
		return len(b), nil
	}
	l.out = append(l.out, b...)
	return len(b), nil
}

// from returns the output after offset.
func (l *liveRun) from(offset int) []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	if offset >= len(l.out) {
		return nil
	}
	return append([]byte(nil), l.out[offset:]...)
}

var (
	liveMu   sync.Mutex
	liveRuns = map[int64]*liveRun{}
)

func teamRunCount(team string) int {
	n := 0
	for _, l := range liveRuns {
		if l.team == team {
			n++
		}
	}
	return n
}

// OneOffRun is a command run once through the API, with how it went.
type OneOffRun struct {
	JobRun
	Command string `json:"command"`
}

// startOneOff runs command once in a container from the app's latest build.
func (p *Panel) startOneOff(src Source, command string, timeout time.Duration) (int64, error) {
	image := "skiff-" + src.App + ":latest"
	if !p.eng.ImageExists(image) {
		return 0, errors.New("deploy the app before running commands on it")
	}
	liveMu.Lock()
	if teamRunCount(src.Team) >= maxTeamRuns {
		liveMu.Unlock()
		return 0, fmt.Errorf("your team already has %d runs going; wait for one to finish", maxTeamRuns)
	}
	res, err := sqlDB.Exec(`INSERT INTO job_runs(job, app, trigger, attempt, started, status, command) VALUES('',?,?,1,?,?,?)`,
		src.App, triggerAPI, time.Now().Unix(), runRunning, command)
	if err != nil {
		liveMu.Unlock()
		return 0, err
	}
	id, _ := res.LastInsertId()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	live := &liveRun{team: src.Team, done: make(chan struct{}), cancel: cancel}
	liveRuns[id] = live
	liveMu.Unlock()
	_, _ = sqlDB.Exec(`DELETE FROM job_runs WHERE job='' AND app=? AND id NOT IN (SELECT id FROM job_runs WHERE job='' AND app=? ORDER BY id DESC LIMIT ?)`,
		src.App, src.App, keepOneOffRuns)

	go guard("run:"+strconv.FormatInt(id, 10), func() {
		defer cancel()
		settings := p.appSettings(src)
		settings.OOMScore = max(settings.OOMScore, oomOneOff)
		err := p.eng.RunStream(ctx, image, command, settings, live)
		status, code := runOK, 0
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			status = runTimeout
			_, _ = fmt.Fprintf(live, "\nStopped after %s, this run's timeout.\n", timeout)
		case errors.Is(ctx.Err(), context.Canceled):
			status = runFailed
			_, _ = fmt.Fprintln(live, "\nStopped by request.")
		case err != nil:
			status = runFailed
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			}
		}
		finishRun(id, status, code, string(live.from(0)))
		close(live.done)
		// Someone following can still read everything for a minute; after that the stored end of the output serves.
		time.AfterFunc(time.Minute, func() {
			liveMu.Lock()
			delete(liveRuns, id)
			liveMu.Unlock()
		})
	})
	return id, nil
}

func getOneOff(app string, id int64) (OneOffRun, bool) {
	var r OneOffRun
	err := sqlDB.QueryRow(`SELECT id, trigger, attempt, started, finished, status, exit_code, output, command FROM job_runs WHERE id=? AND app=? AND job=''`, id, app).
		Scan(&r.ID, &r.Trigger, &r.Attempt, &r.Started, &r.Finished, &r.Status, &r.ExitCode, &r.Output, &r.Command)
	return r, err == nil
}

func listOneOffs(app string, limit int) []OneOffRun {
	rows, err := sqlDB.Query(`SELECT id, trigger, attempt, started, finished, status, exit_code, output, command FROM job_runs WHERE app=? AND job='' ORDER BY id DESC LIMIT ?`, app, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []OneOffRun{}
	for rows.Next() {
		var r OneOffRun
		if rows.Scan(&r.ID, &r.Trigger, &r.Attempt, &r.Started, &r.Finished, &r.Status, &r.ExitCode, &r.Output, &r.Command) == nil {
			out = append(out, r)
		}
	}
	return out
}

func liveFor(id int64) (*liveRun, bool) {
	liveMu.Lock()
	defer liveMu.Unlock()
	l, ok := liveRuns[id]
	return l, ok
}

// apiStartRun starts a one-off run of a command and answers with its id straight away.
func (p *Panel) apiStartRun(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.PathValue("name"))
	src, ok := apiSource(r, app)
	if !ok {
		apiErr(w, http.StatusNotFound, "app not found")
		return
	}
	var body struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body) != nil || strings.TrimSpace(body.Command) == "" {
		apiErr(w, http.StatusBadRequest, "a command is required")
		return
	}
	if problem := checkJobLimits(body.Timeout, 0, ""); problem != "" {
		apiErr(w, http.StatusBadRequest, problem)
		return
	}
	timeout := time.Duration(body.Timeout) * time.Second
	if timeout == 0 {
		timeout = defaultJobTimeout * time.Second
	}
	id, err := p.startOneOff(src, body.Command, timeout)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "runs going") {
			code = http.StatusTooManyRequests
		}
		apiErr(w, code, err.Error())
		return
	}
	recordAudit(apiTeam(r), AuditEntry{Actor: apiActor(r), Action: "run.start", Target: app, Detail: fmt.Sprintf("run %d via api", id)})
	run, _ := getOneOff(app, id)
	apiJSON(w, http.StatusAccepted, run)
}

func (p *Panel) apiListRuns(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.PathValue("name"))
	if _, ok := apiSource(r, app); !ok {
		apiErr(w, http.StatusNotFound, "app not found")
		return
	}
	apiJSON(w, http.StatusOK, listOneOffs(app, keepOneOffRuns))
}

func apiRun(w http.ResponseWriter, r *http.Request) (OneOffRun, bool) {
	app := sanitizeName(r.PathValue("name"))
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, ok := apiSource(r, app); !ok {
		apiErr(w, http.StatusNotFound, "run not found")
		return OneOffRun{}, false
	}
	run, ok := getOneOff(app, id)
	if !ok {
		apiErr(w, http.StatusNotFound, "run not found")
	}
	return run, ok
}

func (p *Panel) apiGetRun(w http.ResponseWriter, r *http.Request) {
	if run, ok := apiRun(w, r); ok {
		apiJSON(w, http.StatusOK, run)
	}
}

// apiRunLogs streams a run's output as plain text until it finishes. Ask for the run afterwards for its exit code.
func (p *Panel) apiRunLogs(w http.ResponseWriter, r *http.Request) {
	run, ok := apiRun(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	live, ok := liveFor(run.ID)
	fl, canFlush := w.(http.Flusher)
	if !ok || !canFlush {
		_, _ = w.Write([]byte(run.Output))
		return
	}
	fl.Flush()
	offset := 0
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		if b := live.from(offset); len(b) > 0 {
			offset += len(b)
			_, _ = w.Write(b)
			fl.Flush()
		}
		select {
		case <-live.done:
			if b := live.from(offset); len(b) > 0 {
				_, _ = w.Write(b)
			}
			return
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

// apiStopRun stops a run that's still going.
func (p *Panel) apiStopRun(w http.ResponseWriter, r *http.Request) {
	run, ok := apiRun(w, r)
	if !ok {
		return
	}
	live, ok := liveFor(run.ID)
	if !ok || run.Status != runRunning {
		apiErr(w, http.StatusConflict, "this run has already finished")
		return
	}
	live.cancel()
	<-live.done
	app := sanitizeName(r.PathValue("name"))
	recordAudit(apiTeam(r), AuditEntry{Actor: apiActor(r), Action: "run.stop", Target: app, Detail: fmt.Sprintf("run %d via api", run.ID)})
	stopped, _ := getOneOff(app, run.ID)
	apiJSON(w, http.StatusOK, stopped)
}
