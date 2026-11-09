package panel

import (
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/seal"
)

func TestBuildArgsAndMountedFilesAreSealed(t *testing.T) {
	openTestDB(t)
	src := Source{App: "api", Team: "t1", Port: "3000", BuildArgs: `["NPM_TOKEN=hunter2"]`, MountedFiles: `[{"to":"/app/.env","content":"U0VDUkVUPTE="}]`}
	if err := putSource(src); err != nil {
		t.Fatal(err)
	}
	var args, files string
	_ = sqlDB.QueryRow(`SELECT build_args, mounted_files FROM sources WHERE app='api'`).Scan(&args, &files)
	if !seal.IsSealed(args) || !seal.IsSealed(files) || strings.Contains(args, "hunter2") {
		t.Fatalf("stored as %q and %q", args, files)
	}
	got, ok := getSource("api")
	if !ok || got.BuildArgs != src.BuildArgs || got.MountedFiles != src.MountedFiles {
		t.Fatalf("read back as %+v", got)
	}
	// Saving what was read doesn't seal it twice.
	if err := putSource(got); err != nil {
		t.Fatal(err)
	}
	if again, _ := getSource("api"); again.BuildArgs != src.BuildArgs {
		t.Fatalf("after a second save: %q", again.BuildArgs)
	}
}
