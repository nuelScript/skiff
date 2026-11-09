package panel

import (
	"os/exec"
	"slices"
	"testing"

	"github.com/nuelScript/skiff/internal/docker"
)

// Changing one worker leaves the app's other workers running, and a worker named queue doesn't count queue-high's containers.
func TestChangingAWorkerLeavesTheOthers(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	openTestDB(t)
	app := "wk" + randToken()[:6]
	team := "wkteam" + randToken()[:4]
	image := "skiff-" + app + ":latest"
	if out, err := exec.Command("docker", "tag", docker.VolumeTool, image).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	p := &Panel{eng: docker.Local()}
	t.Cleanup(func() {
		for _, c := range p.eng.WorkerContainers(app) {
			_ = exec.Command("docker", "rm", "-f", c).Run()
		}
		_ = exec.Command("docker", "rmi", image).Run()
		_ = exec.Command("docker", "network", "rm", teamNetwork(team)).Run()
	})
	if err := putSource(Source{App: app, Team: team, Port: "3000"}); err != nil {
		t.Fatal(err)
	}
	for _, w := range []workerRow{
		{ID: "w1", App: app, Team: team, Name: "queue", Command: "sleep 600", Replicas: 1, Created: 1},
		{ID: "w2", App: app, Team: team, Name: "queue-high", Command: "sleep 600", Replicas: 2, Created: 1},
	} {
		if err := putWorker(w); err != nil {
			t.Fatal(err)
		}
	}
	p.reconcileWorkers(app)
	before := p.eng.WorkerContainers(app)
	if len(before) != 3 {
		t.Fatalf("started %v", before)
	}
	var queue string
	for _, c := range before {
		if replicaOfWorker(c, docker.WorkerPrefix(app, "queue")) {
			queue = c
		}
	}
	if w := toWorker(workerRow{App: app, Name: "queue", Replicas: 1}, before); w.Running != 1 {
		t.Errorf("queue counts %d running", w.Running)
	}

	if err := putWorker(workerRow{ID: "w2", App: app, Team: team, Name: "queue-high", Command: "sleep 601", Replicas: 2, Created: 1}); err != nil {
		t.Fatal(err)
	}
	p.reconcileWorkers(app)
	after := p.eng.WorkerContainers(app)
	if len(after) != 3 || !slices.Contains(after, queue) {
		t.Fatalf("queue's container %s was replaced when only queue-high changed: %v -> %v", queue, before, after)
	}
	for _, c := range before {
		if c != queue && slices.Contains(after, c) {
			t.Errorf("queue-high's old container %s is still there", c)
		}
	}
}
