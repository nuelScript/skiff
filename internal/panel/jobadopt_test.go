package panel

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

// A run that ended while Skiff was down keeps its result: it's recorded as it ended, not retried as unknown.
func TestRunFinishedWhileDownKeepsItsResult(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	openTestDB(t)
	j := jobRow{ID: "nightly", App: "a", Team: "t", Name: "nightly", Command: "true", Retries: 3, Created: 1, LastOk: 1}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	id := startRun(j, triggerSchedule, 1, runRunning)
	name := "skiff-test-run-" + randToken()[:8]
	if out, err := exec.Command("docker", "run", "--name", name, "--label", docker.JobRunLabel+"="+strconv.FormatInt(id, 10), docker.VolumeTool, "sh", "-c", "echo all done").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })

	(&Panel{eng: docker.Local()}).adoptRunsAfterRestart()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if r := listRuns(j.ID, 1)[0]; r.Status != runRunning {
			if r.ID != id || r.Status != runOK || !strings.Contains(r.Output, "all done") {
				t.Fatalf("recorded as %+v", r)
			}
			if exec.Command("docker", "inspect", name).Run() == nil {
				t.Error("the finished container wasn't collected")
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the run was never settled")
}
