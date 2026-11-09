//go:build e2e

package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTimescaleDatabase creates Postgres with TimescaleDB: hypertables and time buckets work, and a backup restores through Timescale's own steps with every chunk back, including after rows written since.
func TestTimescaleDatabase(t *testing.T) {
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
		ID        string
		Engine    string
		Timescale bool
	}
	postJSON(t, c, base+"/api/databases", `{"engine":"timescaledb","name":"metrics"}`, &db)
	ctr := "skiff-db-" + db.ID
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", ctr).Run()
		_ = exec.Command("docker", "volume", "rm", ctr+"-data").Run()
	})
	if db.Engine != "postgres" || !db.Timescale {
		t.Fatalf("created as %+v", db)
	}
	sql := func(stmt string) (string, error) {
		out, err := exec.Command("docker", "exec", ctr, "psql", "-U", "skiff", "-d", "metrics", "-v", "ON_ERROR_STOP=1", "-qtAc", stmt).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	eventually(t, 3*time.Minute, func() bool {
		got, err := sql("select extname from pg_extension where extname='timescaledb'")
		return err == nil && got == "timescaledb"
	})
	if got, err := sql("show shared_buffers"); err != nil || got == "128MB" {
		t.Errorf("shared_buffers: %q %v", got, err)
	}
	if out, err := sql(`create table readings (at timestamptz not null, sensor int, value double precision);
		select create_hypertable('readings', by_range('at', interval '1 day'));
		insert into readings select t, 1, extract(hour from t) from generate_series(now() - interval '5 days', now(), interval '1 hour') t;`); err != nil {
		t.Fatalf("making a hypertable: %v: %s", err, out)
	}
	summary := `select count(*) || '/' || (select count(*) from show_chunks('readings')) || '/' || count(distinct time_bucket('1 day', at)) from readings`
	want, err := sql(summary)
	if err != nil || !strings.HasPrefix(want, "121/") {
		t.Fatalf("summary: %q %v", want, err)
	}

	var b struct{ ID string }
	postJSON(t, c, base+"/api/backups?db="+db.ID, "", &b)
	if out, err := sql("insert into readings values (now() + interval '30 days', 2, 1); delete from readings where sensor = 1 and at < now() - interval '2 days'"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := checkBackup(t, c, panel, db.ID, b.ID); got.CheckStatus != "passed" {
		t.Fatalf("restore check: %+v", got)
	}
	post(t, c, base+"/api/backups/restore?id="+b.ID, "")
	if got, err := sql(summary); err != nil || got != want {
		t.Errorf("after restore: %q %v, want %q", got, err, want)
	}
	// The restored hypertable still takes new rows into new chunks.
	if out, err := sql("insert into readings values (now() + interval '40 days', 3, 1); select count(*) from readings where sensor = 3"); err != nil || !strings.HasSuffix(out, "1") {
		t.Errorf("writing after restore: %v: %s", err, out)
	}
	if got, err := sql("select count(*) from pg_database where datname like '%_restore'"); err != nil || got != "0" {
		t.Errorf("a restore database was left behind: %q %v", got, err)
	}
}
