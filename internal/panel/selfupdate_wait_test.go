package panel

import (
	"testing"
	"time"
)

func TestSelfUpdateWaitsForRunningDeploys(t *testing.T) {
	openTestDB(t)
	addDeploy(Deploy{ID: "d1", App: "www", Status: "building", Started: time.Now().Unix()})
	if _, err := sqlDB.Exec(`UPDATE deploys SET panel_pid=1 WHERE id='d1'`); err != nil {
		t.Fatal(err)
	}
	quiet := func(string, ...any) {}

	start := time.Now()
	go func() {
		time.Sleep(200 * time.Millisecond)
		setDeployStatus("www", "d1", "live")
	}()
	waitForDeploys(10*time.Second, 20*time.Millisecond, quiet)
	if waited := time.Since(start); waited < 150*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("should return once the deploy ends, waited %s", waited)
	}

	addDeploy(Deploy{ID: "d2", App: "www", Status: "building", Started: time.Now().Unix()})
	if _, err := sqlDB.Exec(`UPDATE deploys SET panel_pid=1 WHERE id='d2'`); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	waitForDeploys(100*time.Millisecond, 20*time.Millisecond, quiet)
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("a stuck deploy must not hold the update for ever, waited %s", waited)
	}
}

func TestSelfUpdateWaitsForDiskMoves(t *testing.T) {
	openTestDB(t)
	if !startMove("skiff-wait-test-data", "/mnt/disk") {
		t.Fatal("move didn't start")
	}
	start := time.Now()
	go func() {
		time.Sleep(200 * time.Millisecond)
		endMove("skiff-wait-test-data", nil)
	}()
	waitForDeploys(10*time.Second, 20*time.Millisecond, func(string, ...any) {})
	if waited := time.Since(start); waited < 150*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("should return once the move ends, waited %s", waited)
	}
}
