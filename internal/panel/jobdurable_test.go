package panel

import (
	"os/exec"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

func countRuns(job, status string) int {
	var n int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM job_runs WHERE job=? AND status=?`, job, status).Scan(&n)
	return n
}

// A scheduled run waiting for a slot is already a row, so a restart in the meantime doesn't lose it.
func TestDueRunIsQueuedWhileItWaitsForASlot(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "due", App: "gone", Team: "t", Name: "due", Schedule: "* * * * *", Command: "true", Created: 1, LastOk: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	for range cap(runSlots) {
		runSlots <- struct{}{}
	}
	released := false
	release := func() {
		if !released {
			released = true
			for range cap(runSlots) {
				<-runSlots
			}
		}
	}
	defer release()
	(&Panel{}).runDueJobs(time.Now())
	if n := countRuns(j.ID, runQueued); n != 1 {
		t.Fatalf("%d queued runs while waiting for a slot, want 1", n)
	}
	release()
	deadline := time.Now().Add(5 * time.Second)
	for countRuns(j.ID, runFailed) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := countRuns(j.ID, runQueued); n != 0 {
		t.Fatalf("the queued row was left behind: %d", n)
	}
	if n := countRuns(j.ID, runFailed); n != 1 {
		t.Fatalf("the waiting row became %d failed runs, want the one row", n)
	}
}

// A job deleted while it ran isn't retried or alerted on.
func TestDeletedJobIsNotRetried(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "gone", App: "gone", Team: "t", Name: "gone", Command: "false", Retries: 2, Created: 1, LastOk: 1}
	done := make(chan struct{})
	go func() {
		(&Panel{}).execJobFrom(j, triggerManual, 0, "", 1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("waited to retry a job that no longer exists")
	}
	if n := countRuns(j.ID, runQueued); n != 0 {
		t.Fatalf("%d retries queued for a deleted job", n)
	}
}

// A run that was going when Skiff stopped and has no retries left still tells the team.
func TestInterruptedRunWithoutRetriesFailsTheJob(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	openTestDB(t)
	j := jobRow{ID: "once", App: "a", Team: "t", Name: "once", Command: "true", Created: 1, LastOk: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	startRun(j, triggerSchedule, 1, runRunning)
	(&Panel{eng: docker.Local()}).adoptRunsAfterRestart()
	got, _ := getJob(j.ID)
	if got.LastOk != 0 || got.LastRun == 0 {
		t.Fatalf("job after an interrupted run: ok=%d last run=%d", got.LastOk, got.LastRun)
	}
}

func TestPruningOneOffsSparesLiveRuns(t *testing.T) {
	openTestDB(t)
	ins := func(status string) int64 {
		res, err := sqlDB.Exec(`INSERT INTO job_runs(job, app, trigger, attempt, started, status) VALUES('','a',?,1,1,?)`, triggerAPI, status)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	long := ins(runRunning)
	for range keepOneOffRuns + 5 {
		ins(runOK)
	}
	pruneOneOffs("a")
	var n int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM job_runs WHERE id=?`, long).Scan(&n)
	if n != 1 {
		t.Error("a one-off still going was pruned")
	}
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM job_runs WHERE app='a'`).Scan(&n)
	if n != keepOneOffRuns+1 {
		t.Errorf("%d rows left, want %d", n, keepOneOffRuns+1)
	}
}

// A retry gets the webhook body or file key its first attempt had.
func TestRetryKeepsItsTriggerInput(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "hooked", App: "a", Team: "t", Name: "hooked", Command: "true", Created: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	id := queueRetry(j, 1, `{"n":1}`, triggerWebhook)
	origin := runOrigin(id, triggerRetry)
	env := withTriggerEnv(nil, j, triggerRetry, origin, `{"n":1}`)
	if env["SKIFF_TRIGGER"] != triggerRetry || env["SKIFF_WEBHOOK_BODY"] != `{"n":1}` {
		t.Errorf("env = %v", env)
	}
	if runOrigin(0, triggerBucket) != triggerBucket {
		t.Error("a first attempt's origin is its own trigger")
	}
}

// A CLI app whose settings can't be read doesn't start without its limits.
func TestCLIAppFailsClosedWithoutItsContainer(t *testing.T) {
	openTestDB(t)
	t.Setenv("HOME", t.TempDir())
	src := Source{App: "cliapp", Team: "t", Port: "80", Origin: originCLI}
	if _, err := (&Panel{eng: docker.Local()}).appSettings(src); err == nil {
		t.Fatal("started a CLI app with empty settings")
	}
}
