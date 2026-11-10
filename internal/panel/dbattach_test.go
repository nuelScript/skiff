package panel

import (
	"net/http"
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/docker"
)

// An app gets one database per variable: a second Postgres would overwrite the first's DATABASE_URL.
func TestOneDatabasePerVariable(t *testing.T) {
	openTestDB(t)
	putSource(Source{App: "web", Team: "t1", Port: "3000"})
	for _, d := range []dbRow{
		{ID: "d1", Team: "t1", Name: "orders", Engine: "postgres", Container: "skiff-db-d1"},
		{ID: "d2", Team: "t1", Name: "analytics", Engine: "postgres", Container: "skiff-db-d2"},
		{ID: "d3", Team: "t1", Name: "cache", Engine: "redis", Container: "skiff-db-d3"},
	} {
		if err := putDB(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := attachDB("d1", "web", "DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	p := &Panel{eng: docker.Local()}
	second, _ := getDB("d2")
	err := p.attachDatabase(second, "web")
	if attachStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "orders") {
		t.Fatalf("a second Postgres: %v", err)
	}
	cache, _ := getDB("d3")
	if err := p.attachDatabase(cache, "web"); attachStatus(err) == http.StatusConflict {
		t.Errorf("Redis uses its own variable but was refused: %v", err)
	}
}
