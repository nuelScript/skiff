package panel

import (
	"os/exec"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

// A replica that keeps crashing before it copies says it's failing, and why, instead of showing as copying for ever.
func TestAFailingReplicaSaysWhy(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	openTestDB(t)
	d := dbRow{ID: "rf1", Engine: "postgres", Container: "skiff-db-rftest", Host: "skiff-db-rftest", Port: 5432, Username: "skiff", Password: "pw", DBName: "app"}
	r := replicaRow{ID: "deadbeef", DBID: d.ID, Container: d.Container + "-replica-deadbeef"}
	if _, err := sqlDB.Exec(`INSERT INTO db_replicas(id,db_id,container,created) VALUES(?,?,?,1)`, r.ID, r.DBID, r.Container); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", r.Container).Run() })
	if out, err := exec.Command("docker", "run", "-d", "--name", r.Container, "--restart", "on-failure", docker.VolumeTool, "sh", "-c", "echo 'pg_basebackup: error: could not connect'; exit 1").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	p := &Panel{eng: docker.Local()}
	deadline := time.Now().Add(30 * time.Second)
	var st []ReplicaStatus
	for time.Now().Before(deadline) {
		if st = p.replicaStatus(d); len(st) == 1 && st[0].Phase == "failing" {
			if st[0].Problem != "pg_basebackup: error: could not connect" {
				t.Errorf("problem = %q", st[0].Problem)
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("status %+v", st)
}

// A replica container left without its row, by an add or removal cut off by a restart, is cleaned up.
func TestOrphanReplicasAreRemoved(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	openTestDB(t)
	d := dbRow{ID: "rf2", Engine: "postgres", Container: "skiff-db-rforphan"}
	orphan := d.Container + "-replica-0badf00d"
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", orphan).Run() })
	if out, err := exec.Command("docker", "run", "-d", "--name", orphan, "--label", "skiff.db="+d.ID, docker.VolumeTool, "sleep", "60").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	(&Panel{eng: docker.Local()}).recoverReplicas(d)
	if exec.Command("docker", "inspect", orphan).Run() == nil {
		t.Error("the orphaned replica is still there")
	}
}
