package panel

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/seal"
	"github.com/robfig/cron/v3"
)

type Job struct {
	ID       string `json:"id"`
	App      string `json:"app"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	LastRun  int64  `json:"lastRun"`
	LastOk   bool   `json:"lastOk"`
	Next     int64  `json:"next"`
	Created  int64  `json:"created"`
	// Timeout is in seconds; Retries is how many more times a failed run is tried; CatchUp is "skip" to drop runs missed while Skiff was down, or empty to run once.
	Timeout int    `json:"timeout"`
	Retries int    `json:"retries"`
	CatchUp string `json:"catchUp"`
	Running bool   `json:"running"`
	// Webhook is the address that starts the job when a signed request arrives, with the secret it's signed with.
	Webhook       string `json:"webhook,omitempty"`
	WebhookSecret string `json:"webhookSecret,omitempty"`
	// Bucket and Prefix start the job for each new file under the prefix in one of the team's buckets.
	Bucket string        `json:"bucket,omitempty"`
	Prefix string        `json:"prefix,omitempty"`
	Queued int           `json:"queued"`
	Steps  []config.Step `json:"steps,omitempty"`
}

type jobRow struct {
	ID, App, Team, Name, Schedule, Command string
	LastRun                                int64
	LastOk                                 int
	Created                                int64
	Timeout, Retries                       int
	CatchUp                                string
	// HookSecret signs webhook calls; empty means the job has no webhook. Stored sealed.
	HookSecret string
	// Bucket and Prefix watch a bucket. Seen is the newest file time already handled, and SeenKeys the files at that time, as JSON.
	Bucket, Prefix string
	Seen           int64
	SeenKeys       string
	Steps          []config.Step
}

const jobCols = `id,app,team,name,schedule,command,last_run,last_ok,created,timeout,retries,catch_up,hook_secret,bucket,bucket_prefix,bucket_seen,bucket_seen_keys,steps`

var (
	jobMu      sync.Mutex
	jobRunning = map[string]bool{}
	// jobSkipped remembers the scheduled time last recorded as skipped, so a long run records each overlap once.
	jobSkipped = map[string]int64{}
)

func jobIsRunning(id string) bool {
	jobMu.Lock()
	defer jobMu.Unlock()
	return jobRunning[id]
}

func claimJob(id string) bool {
	jobMu.Lock()
	defer jobMu.Unlock()
	if jobRunning[id] {
		return false
	}
	jobRunning[id] = true
	return true
}

func releaseJob(id string) {
	jobMu.Lock()
	delete(jobRunning, id)
	jobMu.Unlock()
}

func scanJob(row interface{ Scan(...any) error }) (jobRow, bool) {
	var j jobRow
	var steps string
	if row.Scan(&j.ID, &j.App, &j.Team, &j.Name, &j.Schedule, &j.Command, &j.LastRun, &j.LastOk, &j.Created, &j.Timeout, &j.Retries, &j.CatchUp,
		&j.HookSecret, &j.Bucket, &j.Prefix, &j.Seen, &j.SeenKeys, &steps) != nil {
		return jobRow{}, false
	}
	j.Steps = decodeSteps(steps)
	if seal.OpenAll(&j.HookSecret) != nil {
		return jobRow{}, false
	}
	return j, true
}

func getJob(id string) (jobRow, bool) {
	return scanJob(sqlDB.QueryRow(`SELECT `+jobCols+` FROM jobs WHERE id=?`, id))
}

func queryJobs(q string, args ...any) []jobRow {
	rows, err := sqlDB.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []jobRow
	for rows.Next() {
		if j, ok := scanJob(rows); ok {
			out = append(out, j)
		}
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}

func listJobRows(app string) []jobRow {
	return queryJobs(`SELECT `+jobCols+` FROM jobs WHERE app=? ORDER BY created`, app)
}

func allJobRows() []jobRow { return queryJobs(`SELECT ` + jobCols + ` FROM jobs`) }

func putJob(j jobRow) error {
	secret, err := sealHookSecret(j.HookSecret)
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(`INSERT INTO jobs(`+jobCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.App, j.Team, j.Name, j.Schedule, j.Command, j.LastRun, j.LastOk, j.Created, j.Timeout, j.Retries, j.CatchUp,
		secret, j.Bucket, j.Prefix, j.Seen, j.SeenKeys, encodeSteps(j.Steps))
	return err
}

func sealHookSecret(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return seal.Seal(s)
}

func setJobRun(id string, when int64, ok bool) {
	_, _ = sqlDB.Exec(`UPDATE jobs SET last_run=?, last_ok=? WHERE id=?`, when, b2i(ok), id)
}

func toJob(j jobRow) Job {
	var next int64
	if s, err := cron.ParseStandard(j.Schedule); err == nil && j.Schedule != "" {
		base := time.Now()
		if j.LastRun > 0 {
			base = time.Unix(j.LastRun, 0)
		}
		next = s.Next(base).Unix()
	}
	out := Job{
		ID: j.ID, App: j.App, Name: j.Name, Schedule: j.Schedule, Command: j.Command,
		LastRun: j.LastRun, LastOk: j.LastOk != 0, Next: next, Created: j.Created,
		Timeout: int(jobTimeout(j) / time.Second), Retries: j.Retries, CatchUp: j.CatchUp, Running: jobIsRunning(j.ID),
		WebhookSecret: j.HookSecret, Bucket: j.Bucket, Prefix: j.Prefix, Queued: queuedRuns(j.ID), Steps: j.Steps,
	}
	if j.HookSecret != "" {
		out.Webhook = dashboardURL + "/api/hooks/jobs/" + j.ID
	}
	return out
}

// missedAfter is how late a scheduled time can be before it counts as missed, such as while Skiff was down.
const missedAfter = 2 * time.Minute

func (p *Panel) jobLoop() {
	p.adoptRunsAfterRestart()
	time.Sleep(90 * time.Second)
	for {
		guard("jobLoop", func() { p.runDueJobs(time.Now()) })
		time.Sleep(time.Minute)
	}
}

func (p *Panel) runDueJobs(now time.Time) {
	for _, j := range allJobRows() {
		if j.Schedule == "" {
			continue
		}
		sched, err := cron.ParseStandard(j.Schedule)
		if err != nil {
			continue
		}
		base := time.Unix(j.Created, 0)
		if j.LastRun > 0 {
			base = time.Unix(j.LastRun, 0)
		}
		due := sched.Next(base)
		if due.After(now) {
			continue
		}
		trigger := triggerSchedule
		if now.Sub(due) > missedAfter {
			if j.CatchUp == "skip" {
				_, _ = sqlDB.Exec(`UPDATE jobs SET last_run=? WHERE id=?`, now.Unix(), j.ID)
				finishRun(startRun(j, triggerSchedule, 1, runSkipped), runSkipped, 0, "Missed while Skiff wasn't running, and this job skips missed runs.")
				continue
			}
			trigger = triggerCatchUp
		}
		if !claimJob(j.ID) {
			jobMu.Lock()
			first := jobSkipped[j.ID] != due.Unix()
			jobSkipped[j.ID] = due.Unix()
			jobMu.Unlock()
			if first {
				finishRun(startRun(j, triggerSchedule, 1, runSkipped), runSkipped, 0, "The previous run was still going, so this one was skipped.")
			}
			continue
		}
		go func(j jobRow) {
			defer releaseJob(j.ID)
			guard("job:"+j.ID, func() { p.execJob(j, trigger, 0, "") })
		}(j)
	}
}

// runJobNow runs a job once for someone waiting on it, without retries, and returns its output.
func (p *Panel) runJobNow(j jobRow) (string, string, error) {
	out, status, err := p.attemptJob(j, triggerManual, 1, 0, "")
	p.settleJob(j, status, err == nil)
	return out, status, err
}

func (p *Panel) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app := sanitizeName(r.URL.Query().Get("app"))
		if !p.canAccess(r, app) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		out := []Job{}
		for _, j := range listJobRows(app) {
			out = append(out, toJob(j))
		}
		writeJSON(w, out)

	case http.MethodPost:
		app := sanitizeName(r.URL.Query().Get("app"))
		if !p.canAccess(r, app) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if workLocked(app) {
			http.Error(w, tomlOwned, http.StatusBadRequest)
			return
		}
		var body jobInput
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body)
		j, problem := newJob(app, body)
		if problem != "" {
			http.Error(w, problem, http.StatusBadRequest)
			return
		}
		if err := putJob(j); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.audit(r, "job.set", app, j.Name)
		writeJSON(w, toJob(j))

	case http.MethodDelete:
		j, ok := getJob(sanitizeID(r.URL.Query().Get("id")))
		if !ok || !p.canAccess(r, j.App) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if workLocked(j.App) {
			http.Error(w, tomlOwned, http.StatusBadRequest)
			return
		}
		deleteJob(j.ID)
		p.audit(r, "job.delete", j.App, j.Name)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// jobInput is a job as the dashboard and the API send it. Timeout is in seconds.
type jobInput struct {
	Name     string        `json:"name"`
	Schedule string        `json:"schedule"`
	Command  string        `json:"command"`
	Timeout  int           `json:"timeout"`
	Retries  int           `json:"retries"`
	CatchUp  string        `json:"catchUp"`
	Webhook  bool          `json:"webhook"`
	Bucket   string        `json:"bucket"`
	Prefix   string        `json:"prefix"`
	Steps    []config.Step `json:"steps"`
}

// newJob checks a scheduled job's settings, the same for the dashboard and the API.
func newJob(app string, in jobInput) (jobRow, string) {
	schedule, command := strings.TrimSpace(in.Schedule), strings.TrimSpace(in.Command)
	for i := range in.Steps {
		in.Steps[i].Name, in.Steps[i].Command = strings.TrimSpace(in.Steps[i].Name), strings.TrimSpace(in.Steps[i].Command)
	}
	switch {
	case len(in.Steps) > 0 && command != "":
		return jobRow{}, "a job with steps runs their commands, so leave its own command empty"
	case len(in.Steps) == 0 && command == "":
		return jobRow{}, "a command is required"
	}
	if err := config.ValidateSteps(in.Steps); err != nil {
		return jobRow{}, err.Error()
	}
	src, _ := getSource(app)
	bucket, prefix := strings.TrimSpace(in.Bucket), strings.TrimSpace(in.Prefix)
	if problem := checkJobTriggers(src.Team, schedule, in.Webhook, bucket, prefix); problem != "" {
		return jobRow{}, problem
	}
	if problem := checkJobLimits(in.Timeout, in.Retries, in.CatchUp); problem != "" {
		return jobRow{}, problem
	}
	j := jobRow{
		ID: randToken()[:12], App: app, Team: src.Team, Name: cmp.Or(strings.TrimSpace(in.Name), "job"),
		Schedule: schedule, Command: command, LastOk: 1, Created: time.Now().Unix(),
		Timeout: in.Timeout, Retries: in.Retries, CatchUp: in.CatchUp, Bucket: bucket, Prefix: prefix, Steps: in.Steps,
	}
	if in.Webhook {
		j.HookSecret = randToken()
	}
	return j, ""
}

func checkJobLimits(timeout, retries int, catchUp string) string {
	switch {
	case timeout != 0 && (timeout < 60 || timeout > maxJobTimeout):
		return "the timeout must be between 1 minute and 7 days"
	case retries < 0 || retries > maxJobRetries:
		return fmt.Sprintf("retries must be between 0 and %d", maxJobRetries)
	case catchUp != "" && catchUp != "skip":
		return `catch-up must be empty, to run once after downtime, or "skip"`
	}
	return ""
}

func deleteJob(id string) {
	_, _ = sqlDB.Exec(`DELETE FROM job_runs WHERE job=?`, id)
	_, _ = sqlDB.Exec(`DELETE FROM jobs WHERE id=?`, id)
}

// handleJobRuns lists a job's recent runs, newest first.
func (p *Panel) handleJobRuns(w http.ResponseWriter, r *http.Request) {
	j, ok := getJob(sanitizeID(r.URL.Query().Get("id")))
	if !ok || !p.canAccess(r, j.App) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, listRuns(j.ID, keepJobRuns))
}

func (p *Panel) handleJobRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	j, ok := getJob(sanitizeID(r.URL.Query().Get("id")))
	if !ok || !p.canAccess(r, j.App) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !claimJob(j.ID) {
		http.Error(w, "this job is already running", http.StatusConflict)
		return
	}
	defer releaseJob(j.ID)
	p.audit(r, "job.run", j.App, j.Name)
	out, status, err := p.runJobNow(j)
	writeJSON(w, map[string]any{"ok": err == nil, "status": status, "output": out})
}
