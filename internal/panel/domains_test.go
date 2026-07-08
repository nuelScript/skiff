package panel

import (
	"testing"
	"time"
)

// TestDeleteAppDomainsKeepsManaged verifies teardown drops a plain custom domain
// bound to an app but leaves a managed branch domain (parent set) in place, and
// that teamDomains surfaces the parent/branch columns intact.
func TestDeleteAppDomainsKeepsManaged(t *testing.T) {
	openTestDB(t)
	now := time.Now().Unix()
	// Both point at the same preview app: one plain, one branch-managed.
	if _, err := sqlDB.Exec(
		`INSERT INTO domains(host,app,team,parent,branch,created) VALUES('app.example.com','myapp-staging','t1','','',?)`,
		now); err != nil {
		t.Fatalf("insert plain domain: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO domains(host,app,team,parent,branch,created) VALUES('staging.example.com','myapp-staging','t1','myapp','staging',?)`,
		now); err != nil {
		t.Fatalf("insert managed domain: %v", err)
	}

	if !deleteAppDomains("myapp-staging") {
		t.Fatal("expected the plain domain to be deleted (RowsAffected > 0)")
	}

	got := teamDomains("t1")
	if len(got) != 1 {
		t.Fatalf("want 1 surviving domain, got %d: %+v", len(got), got)
	}
	if d := got[0]; d.Host != "staging.example.com" || d.Parent != "myapp" || d.Branch != "staging" {
		t.Fatalf("managed branch domain not preserved intact: %+v", d)
	}

	// A second teardown removes nothing (no plain rows left) — no needless re-mirror.
	if deleteAppDomains("myapp-staging") {
		t.Fatal("expected no further deletions on a managed-only app")
	}
}
