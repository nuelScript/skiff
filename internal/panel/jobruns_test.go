package panel

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
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

	(&Panel{eng: docker.Local()}).deleteAppJobs("a")
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

// A long run outlasting many later ones keeps its row, so it can still finish into the history.
func TestPruningSparesLiveRuns(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "busy", App: "a", Team: "t", Name: "busy", Command: "true", Created: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	long := startRun(j, triggerManual, 1, runRunning)
	waiting := startRun(j, triggerManual, 1, runQueued)
	for range keepJobRuns + 5 {
		id := startRun(j, triggerManual, 1, runRunning)
		finishRun(id, runOK, 0, "")
	}
	for _, id := range []int64{long, waiting} {
		var n int
		_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM job_runs WHERE id=?`, id).Scan(&n)
		if n != 1 {
			t.Errorf("run %d was pruned while it was still live", id)
		}
	}
}

// Deleting a job stops its run there and then, instead of letting it go on to its timeout.
func TestDeletingAJobStopsItsRun(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	openTestDB(t)
	j := jobRow{ID: "doomed", App: "a", Team: "t", Name: "doomed", Command: "sleep 60", Created: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	id := startRun(j, triggerManual, 1, runRunning)
	name := "skiff-test-run-" + randToken()[:8]
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "--label", docker.JobRunLabel+"="+strconv.FormatInt(id, 10), docker.VolumeTool, "sleep", "60").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	(&Panel{eng: docker.Local()}).deleteJob(j.ID)
	if exec.Command("docker", "inspect", name).Run() == nil {
		t.Error("the run's container is still there")
	}
}
