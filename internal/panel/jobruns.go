package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"strconv"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

const (
	defaultJobTimeout = 30 * 60
	maxJobTimeout     = 7 * 24 * 3600
	maxJobRetries     = 10
	keepJobRuns       = 50
	jobOutputLimit    = 32 << 10
)

// How a run started.
const (
	triggerSchedule = "schedule"
	triggerCatchUp  = "catch-up"
	triggerManual   = "manual"
	triggerRetry    = "retry"
)

// How a run ended.
const (
	runRunning = "running"
	runOK      = "ok"
	runFailed  = "failed"
	runTimeout = "timeout"
	runSkipped = "skipped"
)

// JobRun is one attempt at a job, or a scheduled time that was skipped.
type JobRun struct {
	ID       int64        `json:"id"`
	Trigger  string       `json:"trigger"`
	Attempt  int          `json:"attempt"`
	Started  int64        `json:"started"`
	Finished int64        `json:"finished"`
	Status   string       `json:"status"`
	ExitCode int          `json:"exitCode"`
	Output   string       `json:"output"`
	Steps    []stepResult `json:"steps,omitempty"`
}

func startRun(j jobRow, trigger string, attempt int, status string) int64 {
	res, err := sqlDB.Exec(`INSERT INTO job_runs(job, app, trigger, attempt, started, status) VALUES(?,?,?,?,?,?)`,
		j.ID, j.App, trigger, attempt, time.Now().Unix(), status)
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	pruneRuns(j.ID)
	return id
}

func finishRun(id int64, status string, exitCode int, output string) {
	if len(output) > jobOutputLimit {
		output = "…" + output[len(output)-jobOutputLimit:]
	}
	_, _ = sqlDB.Exec(`UPDATE job_runs SET finished=?, status=?, exit_code=?, output=? WHERE id=?`,
		time.Now().Unix(), status, exitCode, output, id)
}

func pruneRuns(job string) {
	// A run still going or waiting its turn is never pruned, however many have started since.
	_, _ = sqlDB.Exec(`DELETE FROM job_runs WHERE job=? AND status NOT IN (?,?) AND id NOT IN (SELECT id FROM job_runs WHERE job=? ORDER BY id DESC LIMIT ?)`, job, runRunning, runQueued, job, keepJobRuns)
}

func listRuns(job string, limit int) []JobRun {
	rows, err := sqlDB.Query(`SELECT id, trigger, attempt, started, finished, status, exit_code, output, steps FROM job_runs WHERE job=? ORDER BY id DESC LIMIT ?`, job, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []JobRun{}
	for rows.Next() {
		var r JobRun
		var steps string
		if rows.Scan(&r.ID, &r.Trigger, &r.Attempt, &r.Started, &r.Finished, &r.Status, &r.ExitCode, &r.Output, &steps) == nil {
			if steps != "" {
				_ = json.Unmarshal([]byte(steps), &r.Steps)
			}
			out = append(out, r)
		}
	}
	return out
}

type orphanRun struct {
	id, started    int64
	job            string
	attempt        int
	trigger, input string
}

// adoptRunsAfterRestart picks up runs that were going when Skiff stopped. A run whose container is still going is followed to its end; the rest are tried again if they have retries left, so a long run resumes from the checkpoints it saved.
// dockerWait is how long taking over runs waits for Docker to answer after a restart.
var dockerWait = 5 * time.Minute

func (p *Panel) adoptRunsAfterRestart() {
	rows, err := sqlDB.Query(`SELECT id, job, attempt, started, trigger, input FROM job_runs WHERE status=?`, runRunning)
	if err != nil {
		return
	}
	var runs []orphanRun
	for rows.Next() {
		var r orphanRun
		if rows.Scan(&r.id, &r.job, &r.attempt, &r.started, &r.trigger, &r.input) == nil {
			runs = append(runs, r)
		}
	}
	rows.Close()
	if len(runs) == 0 {
		return
	}
	// Until Docker answers, every run would look gone and be retried while it's still going.
	for deadline := time.Now().Add(dockerWait); p.eng.Available() != nil && time.Now().Before(deadline); {
		time.Sleep(5 * time.Second)
	}
	live := p.eng.LabelledRuns(docker.JobRunLabel)
	for _, r := range runs {
		containers := live[strconv.FormatInt(r.id, 10)]
		j, ok := getJob(r.job)
		if ok && len(j.Steps) == 0 && len(containers) == 1 && claimJob(j.ID) {
			go func() {
				defer releaseJob(j.ID)
				guard("job:"+j.ID, func() { p.followRun(j, r, containers[0]) })
			}()
			continue
		}
		// A run of steps can't be followed from the middle, so it starts again.
		for _, c := range containers {
			_ = p.eng.Remove(c)
		}
		finishRun(r.id, runFailed, 0, "Skiff restarted while this was running, so its result is unknown.")
		if ok && r.trigger != triggerManual && r.attempt <= j.Retries && claimJob(j.ID) {
			go func() {
				defer releaseJob(j.ID)
				guard("job:"+j.ID, func() { p.execJobFrom(j, triggerRetry, 0, r.input, r.attempt+1) })
			}()
		}
	}
}

// followRun waits for a run still going after a restart, records how it ended, and carries on with its retries.
func (p *Panel) followRun(j jobRow, r orphanRun, container string) {
	timeout := jobTimeout(j)
	ctx, cancel := context.WithTimeout(context.Background(), max(time.Until(time.Unix(r.started, 0).Add(timeout)), time.Second))
	defer cancel()
	var out bytes.Buffer
	code, err := p.eng.Follow(ctx, container, &out)
	text := out.String() + "\n(Skiff restarted during this run; the run kept going.)"
	status := runOK
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		status = runTimeout
		text += fmt.Sprintf("\nStopped after %s, this job's timeout.", timeout)
	case err != nil || code != 0:
		status = runFailed
	}
	finishRun(r.id, status, code, text)
	if status != runOK && r.trigger != triggerManual && r.attempt <= j.Retries {
		p.execJobFrom(j, triggerRetry, waitToRetry(j, r.attempt, r.input), r.input, r.attempt+1)
		return
	}
	p.settleJob(j, status, status == runOK)
}

// stopRuns removes the containers of the runs going that match where, so deleting a job or an app doesn't leave its runs going until their timeouts.
func (p *Panel) stopRuns(where string, arg any) {
	rows, err := sqlDB.Query(`SELECT id FROM job_runs WHERE status=? AND `+where, runRunning, arg)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return
	}
	live := p.eng.LabelledRuns(docker.JobRunLabel)
	for _, id := range ids {
		for _, c := range live[strconv.FormatInt(id, 10)] {
			_ = p.eng.Remove(c)
		}
	}
}

func (p *Panel) deleteAppJobs(app string) {
	p.stopRuns(`app=?`, app)
	_, _ = sqlDB.Exec(`DELETE FROM job_runs WHERE app=?`, app)
	_, _ = sqlDB.Exec(`DELETE FROM jobs WHERE app=?`, app)
}

func jobTimeout(j jobRow) time.Duration {
	if j.Timeout <= 0 {
		return defaultJobTimeout * time.Second
	}
	return time.Duration(j.Timeout) * time.Second
}

// retryWait doubles from 30 seconds, up to 10 minutes.
func retryWait(attempt int) time.Duration {
	d := 30 * time.Second << (attempt - 1)
	if d > 10*time.Minute || d <= 0 {
		return 10 * time.Minute
	}
	return d
}

// attemptJob runs the job once and records it as a run: the queued run id when there is one, otherwise a new run. input is what started it, such as a webhook's body or a file's key.
func (p *Panel) attemptJob(j jobRow, trigger string, attempt int, id int64, input string) (string, string, error) {
	if id == 0 {
		id = startRun(j, trigger, attempt, runRunning)
		if input != "" {
			_, _ = sqlDB.Exec(`UPDATE job_runs SET input=? WHERE id=?`, input, id)
		}
	} else {
		_, _ = sqlDB.Exec(`UPDATE job_runs SET status=?, started=? WHERE id=?`, runRunning, time.Now().Unix(), id)
	}
	src, ok := getSource(j.App)
	if !ok {
		finishRun(id, runFailed, 0, "the app no longer exists")
		return "", runFailed, errors.New("unknown app")
	}
	image := "skiff-" + j.App + ":latest"
	if !p.eng.ImageExists(image) {
		finishRun(id, runFailed, 0, "Deploy the app before running its jobs.")
		return "", runFailed, errors.New("deploy the app before running jobs")
	}
	timeout := jobTimeout(j)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	settings := p.appSettings(src)
	settings.Env = withTriggerEnv(settings.Env, j, trigger, input)
	settings.Labels = maps.Clone(settings.Labels)
	if settings.Labels == nil {
		settings.Labels = map[string]string{}
	}
	settings.Labels[docker.JobRunLabel] = strconv.FormatInt(id, 10)
	settings.OOMScore = max(settings.OOMScore, oomOneOff)
	if len(j.Steps) > 0 {
		out, status, code, results := p.runSteps(ctx, image, j.Steps, settings)
		if b, err := json.Marshal(results); err == nil {
			_, _ = sqlDB.Exec(`UPDATE job_runs SET steps=? WHERE id=?`, string(b), id)
		}
		if status == runTimeout {
			out += fmt.Sprintf("\nStopped after %s, this job's timeout.", timeout)
		}
		finishRun(id, status, code, out)
		if status != runOK {
			return out, status, fmt.Errorf("a step %s", status)
		}
		return out, status, nil
	}
	out, err := p.eng.RunOnce(ctx, image, j.Command, settings)
	status, code := runOK, 0
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		status = runTimeout
		out += fmt.Sprintf("\nStopped after %s, this job's timeout.", timeout)
		err = fmt.Errorf("timed out after %s", timeout)
	case err != nil:
		status = runFailed
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
	}
	finishRun(id, status, code, out)
	return out, status, err
}

// execJob runs a job, retrying as it asks, and tells the team when it starts failing and when it recovers. id is a queued run to use for the first attempt, or 0.
func (p *Panel) execJob(j jobRow, trigger string, id int64, input string) {
	p.execJobFrom(j, trigger, id, input, 1)
}

// execJobFrom is execJob starting at a later attempt, for a run picked up after a restart.
func (p *Panel) execJobFrom(j jobRow, trigger string, id int64, input string, first int) {
	var status string
	var err error
	t := trigger
	for attempt := first; ; attempt++ {
		_, status, err = p.attemptJob(j, t, attempt, id, input)
		if err == nil || attempt > j.Retries {
			break
		}
		id, t = waitToRetry(j, attempt, input), triggerRetry
		// A job deleted while it waited to retry stays deleted.
		if _, ok := getJob(j.ID); !ok {
			return
		}
	}
	p.settleJob(j, status, err == nil)
}

// waitToRetry records the next attempt as a queued run, then waits out the back-off. Should Skiff restart in the meantime, the queue finds the run and starts it.
func waitToRetry(j jobRow, attempt int, input string) int64 {
	id := startRun(j, triggerRetry, attempt+1, runQueued)
	if input != "" {
		_, _ = sqlDB.Exec(`UPDATE job_runs SET input=? WHERE id=?`, input, id)
	}
	time.Sleep(retryWait(attempt))
	return id
}

// settleJob records the outcome and alerts on a change, so a job failing every minute sends one message, not one a minute.
func (p *Panel) settleJob(j jobRow, status string, ok bool) {
	setJobRun(j.ID, time.Now().Unix(), ok)
	wasOK := j.LastOk != 0
	switch {
	case !ok && wasOK:
		why := "failed"
		if status == runTimeout {
			why = "ran past its timeout"
		}
		tries := ""
		if j.Retries > 0 {
			tries = fmt.Sprintf(" on all %d attempts", j.Retries+1)
		}
		go dispatchAlert(alertEvent{Team: j.Team, Kind: "job.failed", App: j.App,
			Title:  fmt.Sprintf("Job %s on %s %s", j.Name, j.App, why),
			Detail: "It " + why + tries + ". Open the app's Jobs tab in the dashboard to see each run's output."})
	case ok && !wasOK && j.LastRun > 0:
		go dispatchAlert(alertEvent{Team: j.Team, Kind: "job.recovered", App: j.App,
			Title: fmt.Sprintf("Job %s on %s works again", j.Name, j.App), Detail: "Its latest run finished cleanly."})
	}
}
