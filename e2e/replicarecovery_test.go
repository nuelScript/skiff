//go:build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestReplicasFollowARecovery returns a database with a read replica to an earlier moment: once the database is on the recovered data, the read address never serves what the recovery undid, and the replica is copied again from the recovered data.
func TestReplicasFollowARecovery(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	protectExisting(t, home)
	panel := freeAddr(t)
	start(t, []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup"}, bin, "panel", "--addr", panel, "--domain", "localhost")
	base := "http://" + panel
	waitHTTP(t, base+"/api/me")
	c := client(t)
	post(t, c, base+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	var db struct{ ID string }
	postJSON(t, c, base+"/api/databases", `{"engine":"postgres","name":"orders"}`, &db)
	ctr := "skiff-db-" + db.ID
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.db="+db.ID+"); docker volume rm -f $(docker volume ls -q --filter name="+ctr+")").Run()
	})
	pass := dbPassword(t, c, base, db.ID)
	sqlVia := func(host, stmt string) (string, error) {
		args := []string{"exec", "-e", "PGPASSWORD=" + pass, ctr, "psql", "-U", "skiff", "-d", "orders"}
		if host != "" {
			args = append(args, "-h", host)
		}
		out, err := exec.Command("docker", append(args, "-v", "ON_ERROR_STOP=1", "-qtAc", stmt)...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	mustSQL := func(stmt string) {
		t.Helper()
		var out string
		var err error
		eventually(t, 2*time.Minute, func() bool {
			out, err = sqlVia("", stmt)
			return err == nil
		})
		if err != nil {
			t.Fatalf("%s: %v: %s", stmt, err, out)
		}
	}
	mustSQL("create table t(v text); insert into t values ('a')")

	pitr := base + "/api/databases/pitr?db=" + db.ID
	var status struct{ WindowStart int64 }
	postJSON(t, c, pitr, `{"action":"enable"}`, &status)
	eventually(t, 2*time.Minute, func() bool {
		getJSON(t, c, pitr, &status)
		return status.WindowStart > 0
	})

	var ignored any
	postJSON(t, c, base+"/api/databases/replicas?id="+db.ID, "", &ignored)
	read := ctr + "-read"
	eventually(t, 4*time.Minute, func() bool {
		got, err := sqlVia(read, "select count(*) from t")
		return err == nil && got == "1"
	})

	time.Sleep(2 * time.Second)
	mid := time.Now().Unix()
	time.Sleep(2 * time.Second)
	mustSQL("insert into t values ('undone')")
	eventually(t, time.Minute, func() bool {
		got, err := sqlVia(read, "select count(*) from t where v = 'undone'")
		return err == nil && got == "1"
	})

	// While the recovery runs, whenever the database no longer has the undone row, the read address mustn't either.
	var done, stale atomic.Bool
	defer done.Store(true)
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		for !done.Load() {
			primary, err := sqlVia("", "select count(*) from t where v = 'undone'")
			if err == nil && primary == "0" {
				if got, err := sqlVia(read, "select count(*) from t where v = 'undone'"); err == nil && got == "1" {
					stale.Store(true)
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()
	postJSON(t, c, pitr, fmt.Sprintf(`{"action":"restore","at":%d}`, mid), &ignored)
	done.Store(true)
	<-watched
	if stale.Load() {
		t.Error("the read address served data the recovery undid")
	}

	mustSQL("insert into t values ('after')")
	// A recovered database keeps the settings that took it back; a replica copied from it mustn't stop and break away at that time.
	eventually(t, 4*time.Minute, func() bool {
		got, err := sqlVia(read, "select string_agg(v, ',' order by v) from t")
		return err == nil && got == "a,after"
	})
}
