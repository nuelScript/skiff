//go:build e2e

package e2e

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestReadReplicas adds read replicas to a Postgres database: each copies the data, follows new writes, refuses its own, and answers at the shared read address. Removing one gives back the history the database kept for it, and deleting the database takes its replicas with it.
func TestReadReplicas(t *testing.T) {
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

	type replica struct{ ID, Name, State string }
	type database struct {
		ID           string
		CanReplicate bool
		Replicas     []replica
		ReadURL      string
	}
	var db database
	postJSON(t, c, base+"/api/databases", `{"engine":"postgres","name":"orders"}`, &db)
	ctr := "skiff-db-" + db.ID
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.db="+db.ID+"); docker volume rm -f $(docker volume ls -q --filter name="+ctr+")").Run()
	})
	if !db.CanReplicate || len(db.Replicas) != 0 {
		t.Fatalf("created as %+v", db)
	}
	pass := dbPassword(t, c, base, db.ID)
	sqlIn := func(container, host, stmt string) (string, error) {
		args := []string{"exec", "-e", "PGPASSWORD=" + pass, container, "psql", "-U", "skiff", "-d", "orders"}
		if host != "" {
			args = append(args, "-h", host)
		}
		args = append(args, "-v", "ON_ERROR_STOP=1", "-qtAc", stmt)
		out, err := exec.Command("docker", args...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	eventually(t, 3*time.Minute, func() bool {
		_, err := sqlIn(ctr, "", "select 1")
		return err == nil
	})
	if out, err := sqlIn(ctr, "", "create table items (id serial primary key, name text); insert into items(name) select 'n' || g from generate_series(1, 500) g"); err != nil {
		t.Fatalf("seeding: %v: %s", err, out)
	}

	postJSON(t, c, base+"/api/databases/replicas?id="+db.ID, "", &db)
	if len(db.Replicas) != 1 || !strings.Contains(db.ReadURL, "@"+ctr+"-read:5432/orders") {
		t.Fatalf("after adding a replica: %+v", db)
	}
	first := db.Replicas[0]
	// A replica still copying isn't under the read name yet, so no client is sent to one that would refuse it.
	if out, err := exec.Command("docker", "exec", ctr, "getent", "hosts", ctr+"-read").CombinedOutput(); err == nil {
		t.Errorf("the read name answered while the replica was copying: %s", out)
	}
	streaming := func(n int) bool {
		var st []struct{ ID, Phase string }
		getJSON(t, c, base+"/api/databases/replicas?id="+db.ID, &st)
		got := 0
		for _, s := range st {
			if s.Phase == "streaming" {
				got++
			}
		}
		return got == n
	}
	eventually(t, 4*time.Minute, func() bool { return streaming(1) })
	if got, err := sqlIn(first.Name, "", "select count(*) from items"); err != nil || got != "500" {
		t.Fatalf("replica copy: %q %v", got, err)
	}

	if out, err := sqlIn(ctr, "", "insert into items(name) values ('late')"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	eventually(t, time.Minute, func() bool {
		got, err := sqlIn(first.Name, "", "select count(*) from items where name = 'late'")
		return err == nil && got == "1"
	})
	if out, err := sqlIn(first.Name, "", "insert into items(name) values ('nope')"); err == nil || !strings.Contains(out, "read-only") {
		t.Errorf("writing to a replica: %v: %s", err, out)
	}
	// The shared read address reaches a replica, never the database itself.
	eventually(t, time.Minute, func() bool {
		got, err := sqlIn(ctr, ctr+"-read", "select pg_is_in_recovery()")
		return err == nil && got == "t"
	})

	postJSON(t, c, base+"/api/databases/replicas?id="+db.ID, "", &db)
	if len(db.Replicas) != 2 {
		t.Fatalf("after a second replica: %+v", db)
	}
	eventually(t, 4*time.Minute, func() bool { return streaming(2) })
	if got, err := sqlIn(ctr, "", "select count(*) from pg_replication_slots"); err != nil || got != "2" {
		t.Errorf("slots with two replicas: %q %v", got, err)
	}

	req, _ := http.NewRequest(http.MethodDelete, base+"/api/databases/replicas?id="+db.ID+"&replica="+first.ID, nil)
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("removing a replica: %v %v", resp, err)
	}
	resp.Body.Close()
	if exec.Command("docker", "inspect", first.Name).Run() == nil {
		t.Errorf("%s is still there", first.Name)
	}
	if got, err := sqlIn(ctr, "", "select count(*) from pg_replication_slots"); err != nil || got != "1" {
		t.Errorf("slots after removing one: %q %v", got, err)
	}
	eventually(t, time.Minute, func() bool {
		got, err := sqlIn(ctr, ctr+"-read", "select count(*) from items")
		return err == nil && got == "501"
	})

	req, _ = http.NewRequest(http.MethodDelete, base+"/api/databases?id="+db.ID, nil)
	if resp, err = c.Do(req); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	out, _ := exec.Command("docker", "ps", "-aq", "--filter", "label=skiff.db="+db.ID).Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("containers left after deleting the database: %s", out)
	}
}

// dbPassword reads the database's password from its connection URL.
func dbPassword(t *testing.T, c *http.Client, base, id string) string {
	t.Helper()
	var dbs []struct{ ID, URL string }
	getJSON(t, c, base+"/api/databases", &dbs)
	for _, d := range dbs {
		if d.ID == id {
			rest := d.URL[strings.Index(d.URL, "://")+3:]
			return rest[strings.Index(rest, ":")+1 : strings.Index(rest, "@")]
		}
	}
	t.Fatalf("no database %s", id)
	return ""
}
