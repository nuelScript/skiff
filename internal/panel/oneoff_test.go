package panel

import (
	"bytes"
	"strings"
	"testing"
)

func TestLiveRunKeepsTheEnd(t *testing.T) {
	l := &liveRun{done: make(chan struct{})}
	chunk := bytes.Repeat([]byte("x"), 1<<20)
	for range 6 {
		_, _ = l.Write(chunk)
	}
	_, _ = l.Write([]byte("last line"))
	got := l.tail()
	if !strings.HasSuffix(got, "last line") || !strings.HasPrefix(got, "[earlier output") {
		t.Errorf("tail starts %q ends %q", got[:30], got[len(got)-20:])
	}
	if len(got) > liveOutputLimit+liveOutputSlack+100 {
		t.Errorf("kept %d bytes", len(got))
	}
	b, next := l.from(0)
	if next != 6<<20+len("last line") || !bytes.HasSuffix(b, []byte("last line")) {
		t.Errorf("a reader that fell behind got %d bytes, next %d", len(b), next)
	}
	if b, _ := l.from(next); b != nil {
		t.Error("nothing is left after the end")
	}
}

func TestFinishedRunsFreeTheirSlot(t *testing.T) {
	live := &liveRun{team: "t", done: make(chan struct{})}
	liveMu.Lock()
	liveRuns[-1] = live
	liveMu.Unlock()
	defer func() {
		liveMu.Lock()
		delete(liveRuns, -1)
		liveMu.Unlock()
	}()
	count := func() int {
		liveMu.Lock()
		defer liveMu.Unlock()
		return teamRunCount("t")
	}
	if count() != 1 {
		t.Fatal("a running run should hold a slot")
	}
	close(live.done)
	if count() != 0 {
		t.Error("a finished run still holds a slot")
	}
}
