package docker

import "testing"

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
