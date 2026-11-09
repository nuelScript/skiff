package panel

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const (
	defaultJobTimeout = 30 * 60
	maxJobTimeout     = 24 * 3600
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
	ID       int64  `json:"id"`
	Trigger  string `json:"trigger"`
	Attempt  int    `json:"attempt"`
	Started  int64  `json:"started"`
	Finished int64  `json:"finished"`
	Status   string `json:"status"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
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
	_, _ = sqlDB.Exec(`DELETE FROM job_runs WHERE job=? AND id NOT IN (SELECT id FROM job_runs WHERE job=? ORDER BY id DESC LIMIT ?)`, job, job, keepJobRuns)
}

func listRuns(job string, limit int) []JobRun {
	rows, err := sqlDB.Query(`SELECT id, trigger, attempt, started, finished, status, exit_code, output FROM job_runs WHERE job=? ORDER BY id DESC LIMIT ?`, job, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []JobRun{}
	for rows.Next() {
		var r JobRun
		if rows.Scan(&r.ID, &r.Trigger, &r.Attempt, &r.Started, &r.Finished, &r.Status, &r.ExitCode, &r.Output) == nil {
			out = append(out, r)
		}
	}
	return out
}

// settleRunsAfterRestart marks runs the panel was watching when it stopped, since nothing will finish them now.
func settleRunsAfterRestart() {
	_, _ = sqlDB.Exec(`UPDATE job_runs SET status=?, finished=?, output=output || ? WHERE status=?`,
		runFailed, time.Now().Unix(), "\nSkiff restarted while this was running, so its result is unknown.", runRunning)
}

func deleteAppJobs(app string) {
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
	var status string
	var err error
	for attempt := 1; ; attempt++ {
		t := trigger
		if attempt > 1 {
			t, id = triggerRetry, 0
		}
		_, status, err = p.attemptJob(j, t, attempt, id, input)
		if err == nil || attempt > j.Retries {
			break
		}
		time.Sleep(retryWait(attempt))
	}
	p.settleJob(j, status, err == nil)
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
