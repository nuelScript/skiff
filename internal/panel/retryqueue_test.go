package panel

import (
	"testing"
	"time"
)

// A retry waiting out its back-off is already a queued run, so a restart in the meantime doesn't lose it.
func TestARetryIsQueuedWhileItWaits(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "flaky", App: "gone", Team: "t", Name: "flaky", Command: "false", Retries: 2, Created: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	go waitToRetry(j, 1, `{"n":1}`, triggerBucket)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var attempt int
		var trigger, input string
		if sqlDB.QueryRow(`SELECT attempt, trigger, input FROM job_runs WHERE job=? AND status=?`, j.ID, runQueued).Scan(&attempt, &trigger, &input) == nil {
			if attempt != 2 || trigger != triggerRetry || input != `{"n":1}` {
				t.Fatalf("queued as attempt %d, %s, %q", attempt, trigger, input)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no queued run while the retry waits")
}

// After a restart, a queued retry carries on from its own attempt: the last one allowed ends the job's run of failures instead of starting the retries again.
func TestAQueuedRetryKeepsItsAttempt(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "flaky", App: "gone", Team: "t", Name: "flaky", Command: "false", Retries: 2, Created: 1, LastOk: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	startRun(j, triggerRetry, 3, runQueued)
	(&Panel{}).dispatchQueued()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := getJob(j.ID); got.LastRun > 0 {
			if got.LastOk != 0 {
				t.Errorf("the job's last attempt failed but it reads as ok")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the last attempt started its retries over instead of ending them")
}
