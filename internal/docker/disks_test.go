package docker

import (
	"errors"
	"strings"
	"testing"
)

func TestCopyScriptKeepsRootFolder(t *testing.T) {
	for _, want := range []string{"cp -a /from/. /to/", `chown "$(stat -c %u:%g /from)" /to`, `chmod "$(stat -c %a /from)" /to`} {
		if !strings.Contains(copyScript, want) {
			t.Errorf("copy script is missing %q", want)
		}
	}
}

func TestOnlyItsOwnFolderIsCleared(t *testing.T) {
	cases := []struct {
		folder, name string
		want         bool
	}{
		{"/mnt/data/skiff-volumes/skiff-db-1-data", "skiff-db-1-data", true},
		{"/mnt/data/skiff-volumes/other", "skiff-db-1-data", false},
		{"/home/me/data", "data", false},
		{"/", "x", false},
	}
	for _, c := range cases {
		if got := ownFolder(c.folder, c.name); got != c.want {
			t.Errorf("ownFolder(%q, %q) = %v", c.folder, c.name, got)
		}
	}
}

func TestClearFolderRefusesAFolderOnTheSystemDisk(t *testing.T) {
	err := (&Engine{}).ClearFolder("/no-such-mount/skiff-volumes/v")
	if !errors.Is(err, ErrDiskNotMounted) {
		t.Errorf("err = %v, want ErrDiskNotMounted", err)
	}
}
