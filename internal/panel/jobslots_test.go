package panel

import (
	"testing"
	"time"
)

// With every slot taken, a run waits its turn instead of starting beside the others.
func TestRunsWaitForASlot(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "later", App: "gone", Team: "t", Name: "later", Command: "true", Created: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	for range cap(runSlots) {
		runSlots <- struct{}{}
	}
	done := make(chan struct{})
	go func() {
		_, _, _ = (&Panel{}).attemptJob(j, triggerManual, 1, 0, "")
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("a run started with every slot taken")
	case <-time.After(300 * time.Millisecond):
	}
	if n := len(listRuns(j.ID, 10)); n != 0 {
		t.Errorf("a waiting run was recorded as started: %d rows", n)
	}
	for range cap(runSlots) {
		<-runSlots
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the run never got a slot")
	}
}
