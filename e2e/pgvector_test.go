//go:build e2e

package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPgvectorDatabase creates Postgres with pgvector from the dashboard's API: the extension is on, a nearest-neighbour query works, and a backup restores with the vectors intact.
func TestPgvectorDatabase(t *testing.T) {
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

	var db struct {
		ID     string
		Engine string
		Vector bool
	}
	postJSON(t, c, base+"/api/databases", `{"engine":"pgvector","name":"memory"}`, &db)
	ctr := "skiff-db-" + db.ID
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", ctr).Run()
		_ = exec.Command("docker", "volume", "rm", ctr+"-data").Run()
	})
	if db.Engine != "postgres" || !db.Vector {
		t.Fatalf("created as %+v", db)
	}
	sql := func(stmt string) (string, error) {
		out, err := exec.Command("docker", "exec", ctr, "psql", "-U", "skiff", "-d", "memory", "-v", "ON_ERROR_STOP=1", "-qtAc", stmt).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	// Skiff switches the extension on once the database is up.
	eventually(t, 3*time.Minute, func() bool {
		got, err := sql("select extname from pg_extension where extname='vector'")
		return err == nil && got == "vector"
	})
	// Skiff sizes Postgres to the container instead of its 128 MB default.
	if got, err := sql("show shared_buffers"); err != nil || got == "128MB" {
		t.Errorf("shared_buffers: %q %v", got, err)
	}
	if out, _ := exec.Command("docker", "inspect", "-f", "{{.HostConfig.ShmSize}}", ctr).Output(); strings.TrimSpace(string(out)) == "67108864" {
		t.Error("/dev/shm is still Docker's 64 MB")
	}
	if out, err := sql(`create table notes (id int, body text, embedding vector(3));
		insert into notes values (1, 'cats', '[1,0,0]'), (2, 'dogs', '[0.9,0.1,0]'), (3, 'taxes', '[0,0,1]');
		create index on notes using hnsw (embedding vector_l2_ops);`); err != nil {
		t.Fatalf("using vectors: %v: %s", err, out)
	}
	nearest := `select string_agg(body, ',' order by embedding <-> '[1,0,0]') from (select * from notes order by embedding <-> '[1,0,0]' limit 2) n`
	if got, err := sql(nearest); err != nil || got != "cats,dogs" {
		t.Fatalf("nearest neighbours: %q %v", got, err)
	}

	var b struct{ ID string }
	postJSON(t, c, base+"/api/backups?db="+db.ID, "", &b)
	if out, err := sql("delete from notes"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := checkBackup(t, c, panel, db.ID, b.ID); got.CheckStatus != "passed" {
		t.Fatalf("restore check: %+v", got)
	}
	post(t, c, base+"/api/backups/restore?id="+b.ID, "")
	if got, err := sql(nearest); err != nil || got != "cats,dogs" {
		t.Errorf("after restore: %q %v", got, err)
	}
}
