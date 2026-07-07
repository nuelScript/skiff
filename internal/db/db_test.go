package db

import (
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
