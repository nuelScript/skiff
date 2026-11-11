package panel

import "testing"

func TestImageFlavorsAreNotAdopted(t *testing.T) {
	cases := []struct {
		d       dbRow
		adopted bool
	}{
		{dbRow{Engine: "postgres"}, false},
		{dbRow{Engine: "postgres", Image: pgvectorImage}, false},
		{dbRow{Engine: "postgres", Image: timescaleImage}, false},
		{dbRow{Engine: "postgres", Image: "postgis/postgis:16-3.4"}, true},
		{dbRow{Engine: "mysql", Image: "mysql:8.4"}, true},
		{dbRow{Engine: "postgres", Image: "timescale/timescaledb:2.17.0-pg16"}, false},
		{dbRow{Engine: "postgres", Image: "pgvector/pgvector:pg17", Cmd: `["postgres"]`}, true},
		{dbRow{Engine: "postgres", Image: pgvectorImage, Cmd: "null", Stack: "s1"}, true},
		{dbRow{Engine: "postgres", Image: timescaleImage, Stack: "s1"}, true},
		{dbRow{Engine: "postgres", Image: "registry.local:5000/pgvector/pgvector:pg16"}, true},
	}
	for _, c := range cases {
		if got := c.d.adopted(); got != c.adopted {
			t.Errorf("%s adopted = %v", c.d.Image, got)
		}
		if c.d.Engine == "postgres" && tunesPostgres(c.d) == c.adopted {
			t.Errorf("%s tuned = %v", c.d.Image, !c.adopted)
		}
	}
}

func TestImageRepo(t *testing.T) {
	for in, want := range map[string]string{
		"pgvector/pgvector:pg16":            "pgvector/pgvector",
		"registry.local:5000/team/pg":       "registry.local:5000/team/pg",
		"registry.local:5000/team/pg:16":    "registry.local:5000/team/pg",
		"postgres@sha256:abc":               "postgres",
		"timescale/timescaledb:2.30.2-pg16": "timescale/timescaledb",
	} {
		if got := imageRepo(in); got != want {
			t.Errorf("imageRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFlavorExtensionFollowsTheImage(t *testing.T) {
	for image, want := range map[string]string{pgvectorImage: "vector", timescaleImage: "timescaledb", "postgres:16": "", "": ""} {
		if got := flavorExtension(image); got != want {
			t.Errorf("%q: got %q, want %q", image, got, want)
		}
	}
}

func TestFlavorByRepoIgnoresTag(t *testing.T) {
	if flavorExtension("timescale/timescaledb:2.17.0-pg16") != "timescaledb" || flavorExtension("pgvector/pgvector:pg17") != "vector" {
		t.Error("an older tag lost its extension")
	}
	if !isTimescale("timescale/timescaledb:2.17.0-pg16") || isTimescale(pgvectorImage) || isTimescale("") {
		t.Error("isTimescale")
	}
}

func TestRestoreTimeoutScalesWithSize(t *testing.T) {
	if restoreTimeout(0) != backupTimeout(0) || restoreTimeout(100<<30) <= restoreTimeout(1<<30) {
		t.Error("a restore's limit doesn't grow with the backup")
	}
}
