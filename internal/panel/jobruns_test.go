package panel

import (
	"github.com/nuelScript/skiff/internal/docker"
	"strings"
	"testing"
	"time"
)

func TestJobSchedulingRules(t *testing.T) {
	openTestDB(t)
	p := &Panel{}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	hourAgo := now.Add(-time.Hour).Unix()

	// Missed while Skiff was down, by a job that skips missed runs: recorded as skipped, and the schedule moves on.
	skip := jobRow{ID: "skipper", App: "a", Team: "t", Name: "skipper", Schedule: "*/5 * * * *", Command: "true", LastOk: 1, LastRun: hourAgo, Created: hourAgo, CatchUp: "skip"}
	if err := putJob(skip); err != nil {
		t.Fatal(err)
	}
	// Due while its last run is still going.
	busy := jobRow{ID: "busy", App: "a", Team: "t", Name: "busy", Schedule: "* * * * *", Command: "true", LastOk: 1, LastRun: now.Add(-90 * time.Second).Unix(), Created: hourAgo}
	if err := putJob(busy); err != nil {
		t.Fatal(err)
	}
	if !claimJob(busy.ID) {
		t.Fatal("couldn't claim")
	}
	defer releaseJob(busy.ID)

	p.runDueJobs(now)
	p.runDueJobs(now.Add(10 * time.Second))

	if r := listRuns(skip.ID, 10); len(r) != 1 || r[0].Status != runSkipped || !strings.Contains(r[0].Output, "Missed") {
		t.Errorf("skipped catch-up: %+v", r)
	}
	if j, _ := getJob(skip.ID); j.LastRun != now.Unix() || j.LastOk != 1 {
		t.Errorf("the skipped job should move on without counting as failed: %+v", j)
	}
	if r := listRuns(busy.ID, 10); len(r) != 1 || r[0].Status != runSkipped || !strings.Contains(r[0].Output, "still going") {
		t.Errorf("an overlap should be recorded once: %+v", r)
	}
}

func TestJobRunHistoryIsKeptShort(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "j", App: "a", Name: "j"}
	for i := range keepJobRuns + 5 {
		finishRun(startRun(j, triggerSchedule, 1, runRunning), runOK, 0, strings.Repeat("x", i))
	}
	if n := len(listRuns(j.ID, 1000)); n != keepJobRuns {
		t.Errorf("kept %d runs, want %d", n, keepJobRuns)
	}
	finishRun(startRun(j, triggerManual, 1, runRunning), runOK, 0, strings.Repeat("y", jobOutputLimit+100))
	if out := listRuns(j.ID, 1)[0].Output; len(out) > jobOutputLimit+len("…") || !strings.HasSuffix(out, "y") {
		t.Errorf("output should keep its end, %d bytes", len(out))
	}

	running := startRun(j, triggerSchedule, 1, runRunning)
	(&Panel{eng: docker.Local()}).adoptRunsAfterRestart()
	if r := listRuns(j.ID, 1)[0]; r.ID != running || r.Status != runFailed || !strings.Contains(r.Output, "restarted") {
		t.Errorf("a run cut off by a restart: %+v", r)
	}

	deleteAppJobs("a")
	if n := len(listRuns(j.ID, 1000)); n != 0 {
		t.Errorf("deleting the app left %d runs", n)
	}
}

func TestJobLimits(t *testing.T) {
	for _, c := range []struct {
		timeout, retries int
		catchUp          string
		ok               bool
	}{
		{0, 0, "", true}, {60, 10, "skip", true}, {86400, 3, "", true},
		{59, 0, "", false}, {7*86400 + 1, 0, "", false}, {0, 11, "", false}, {0, -1, "", false}, {0, 0, "always", false},
	} {
		if got := checkJobLimits(c.timeout, c.retries, c.catchUp) == ""; got != c.ok {
			t.Errorf("%+v: accepted = %v", c, got)
		}
	}
	if retryWait(1) != 30*time.Second || retryWait(2) != time.Minute || retryWait(10) != 10*time.Minute {
		t.Errorf("waits: %v %v %v", retryWait(1), retryWait(2), retryWait(10))
	}
}
