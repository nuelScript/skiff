package panel

import (
	"strings"
	"testing"
)

func TestReplicasSupported(t *testing.T) {
	cases := []struct {
		d    dbRow
		want bool
	}{
		{dbRow{Engine: "postgres"}, true},
		{dbRow{Engine: "postgres", Image: pgvectorImage}, true},
		{dbRow{Engine: "postgres", Image: timescaleImage}, true},
		{dbRow{Engine: "postgres", Image: "postgis/postgis:16-3.4"}, false},
		{dbRow{Engine: "postgres", Stack: "shop"}, false},
		{dbRow{Engine: "postgres", PreviewApp: "web-feat"}, false},
		{dbRow{Engine: "mysql"}, false},
	}
	for _, c := range cases {
		if got := replicasSupported(c.d); got != c.want {
			t.Errorf("replicasSupported(%+v) = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestReadURLUsesTheSharedName(t *testing.T) {
	p := &Panel{}
	d := dbRow{Engine: "postgres", Container: "skiff-db-abc", Host: "skiff-db-abc", Port: 5432, Username: "skiff", Password: "pw", DBName: "app"}
	if got, want := p.readURL(d), "postgres://skiff:pw@skiff-db-abc-read:5432/app?sslmode=disable"; got != want {
		t.Fatalf("readURL = %q, want %q", got, want)
	}
	r := replicaRow{ID: "1a2b3c4d", Container: "skiff-db-abc-replica-1a2b3c4d"}
	if r.slot() != "skiff_1a2b3c4d" || r.volume() != "skiff-db-abc-replica-1a2b3c4d-data" {
		t.Fatalf("slot %q volume %q", r.slot(), r.volume())
	}
}

func TestPreviewReadsItsOwnCopy(t *testing.T) {
	openTestDB(t)
	parent := Source{App: "web", Team: "t1", Port: "3000", PreviewData: previewDataCopy}
	preview := Source{App: "web-feat", Team: "t1", Port: "3000", Parent: "web"}
	putSource(parent)
	putSource(preview)
	copyDB := dbRow{ID: "c1", Team: "t1", Name: "web-feat-main", Engine: "postgres", Container: "skiff-db-c1", Host: "skiff-db-c1", Port: 5432, Username: "skiff", Password: "pw", DBName: "main", PreviewApp: "web-feat"}
	if err := putDB(copyDB); err != nil {
		t.Fatal(err)
	}
	if err := attachDB("c1", "web-feat", "DATABASE_URL"); err != nil {
		t.Fatal(err)
	}

	// Without a read address on the project there's nothing to override.
	if err := previewReads(preview, parent); err != nil {
		t.Fatal(err)
	}
	for _, e := range getEnv("web-feat") {
		if e.Key == replicaEnvVar {
			t.Fatalf("set %s for a project without replicas", replicaEnvVar)
		}
	}

	if err := upsertEnvVar("web", replicaEnvVar, "postgres://skiff:pw@skiff-db-main-read:5432/main?sslmode=disable", false); err != nil {
		t.Fatal(err)
	}
	if err := previewReads(preview, parent); err != nil {
		t.Fatal(err)
	}
	var got string
	for _, e := range deployEnv(preview) {
		if e.Key == replicaEnvVar {
			got = e.Value
		}
	}
	if !strings.Contains(got, "@skiff-db-c1:5432/") {
		t.Fatalf("preview reads from %q, want its own copy", got)
	}
}
