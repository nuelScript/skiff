package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestOpenAtIdempotent guards the migration path: opening an existing database
// re-runs every additive migration against columns that already exist, so the
// "duplicate column name" errors must be tolerated while a real failure surfaces.
func TestOpenAtIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiff.db")

	d1, err := OpenAt(path)
	if err != nil {
		t.Fatalf("first OpenAt: %v", err)
	}
	d1.Close()

	// Re-open the populated file: schema re-applies and every ADD COLUMN migration
	// re-runs against columns that now exist — this must not error.
	d2, err := OpenAt(path)
	if err != nil {
		t.Fatalf("re-open (migrations re-run) failed: %v", err)
	}
	defer d2.Close()

	// A column introduced by a migration must be usable after the re-run.
	if _, err := d2.Exec(`INSERT INTO sources(app, scale_cpu) VALUES('t', 50)`); err != nil {
		t.Fatalf("migrated column not usable: %v", err)
	}
}

// TestDomainsMigrationOnOldTable proves the parent/branch columns are added to a
// domains table that predates them (the real upgrade path), leaving existing
// rows intact — the schema's CREATE TABLE IF NOT EXISTS can't do this, only the
// ALTER TABLE migration can.
func TestDomainsMigrationOnOldTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skiff.db")

	// Simulate a pre-upgrade install: a domains table without parent/branch.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE domains (
	  host TEXT PRIMARY KEY, app TEXT NOT NULL, team TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create old domains table: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO domains(host,app,team,created) VALUES('acme.com','api','t1',1)`); err != nil {
		t.Fatalf("seed old row: %v", err)
	}
	raw.Close()

	// Upgrade: OpenAt applies the schema (no-op for the existing table) then the
	// additive migrations, which must add parent/branch.
	d, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt (upgrade): %v", err)
	}
	defer d.Close()

	// The new columns exist, default to empty, and the pre-existing row survived.
	var host, parent, branch string
	if err := d.QueryRow(`SELECT host, parent, branch FROM domains WHERE host='acme.com'`).
		Scan(&host, &parent, &branch); err != nil {
		t.Fatalf("select migrated columns: %v", err)
	}
	if host != "acme.com" || parent != "" || branch != "" {
		t.Fatalf("row not preserved through migration: host=%q parent=%q branch=%q", host, parent, branch)
	}

	// A managed branch domain is now insertable against the upgraded table.
	if _, err := d.Exec(
		`INSERT INTO domains(host,app,team,parent,branch,created) VALUES('staging.acme.com','api-staging','t1','api','staging',2)`); err != nil {
		t.Fatalf("insert managed branch domain after migration: %v", err)
	}
}
