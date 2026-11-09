package panel

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOnlyOnePanelRunsBackgroundWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.lock")
	old, err := holdPanelLock(path, func() { t.Error("the first panel had to wait") })
	if err != nil {
		t.Fatal(err)
	}

	waited := make(chan struct{}, 1)
	got := make(chan error, 1)
	go func() {
		f, err := holdPanelLock(path, func() { waited <- struct{}{} })
		if f != nil {
			defer f.Close()
		}
		got <- err
	}()
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("the new panel didn't wait for the old one")
	}
	select {
	case <-got:
		t.Fatal("the new panel started while the old one held the lock")
	case <-time.After(200 * time.Millisecond):
	}

	old.Close() // the old panel's process exits
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the new panel never started after the old one stopped")
	}
}
