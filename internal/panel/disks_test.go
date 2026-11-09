package panel

import (
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/config"
)

func TestDiskProblem(t *testing.T) {
	openTestDB(t)
	dir := t.TempDir()
	for path, want := range map[string]string{
		"relative/dir":   "absolute folder",
		"/":              "absolute folder",
		dir + "/":        "absolute folder",
		dir + "/missing": "isn't a folder",
		"/tmp/../etc":    "absolute folder",
	} {
		if got := diskProblem(path); !strings.Contains(got, want) {
			t.Errorf("%s: %q, want it to mention %q", path, got, want)
		}
	}
	// A temporary folder is on the system's own disk on most machines, which isn't an added disk.
	if got := diskProblem(dir); got != "" && !strings.Contains(got, "same disk as the system") {
		t.Errorf("a folder on the system disk: %q", got)
	}
	_, _ = sqlDB.Exec(`INSERT INTO disks(id, name, path, created) VALUES('d1', 'data-1', '/mnt/data', 1)`)
	if got := diskProblem("/mnt/data/inner"); got == "" {
		t.Error("a folder inside a listed disk was accepted")
	}
}

func TestDiskOfAndFolders(t *testing.T) {
	disks := []disk{{ID: "d1", Path: "/mnt/data"}, {ID: "d2", Path: "/mnt/data2"}}
	if got := diskOf(config.DiskFolder("/mnt/data2", "skiff-shop_files"), disks); got != "d2" {
		t.Errorf("diskOf: %q", got)
	}
	if got := diskOf("/var/lib/docker/volumes/x", disks); got != "" {
		t.Errorf("Docker's storage: %q", got)
	}
	if got := config.DiskFolder("/mnt/data/", "v"); got != "/mnt/data/skiff-volumes/v" {
		t.Errorf("folder: %q", got)
	}
	for disk, ok := range map[string]bool{"/mnt/data": true, "mnt/data": false, "/mnt/data/": false, "/mnt/../etc": false} {
		err := config.ValidateVolumes([]config.Volume{{Name: "files", Path: "/data", Disk: disk}})
		if (err == nil) != ok {
			t.Errorf("disk %q: %v", disk, err)
		}
	}
}

func TestVolumesRememberTheirDisk(t *testing.T) {
	openTestDB(t)
	_, _ = sqlDB.Exec(`INSERT INTO disks(id, name, path, created) VALUES('d1', 'data-1', '/mnt/data', 1)`)
	if err := addVolumeOn("shop", config.Volume{Name: "files", Path: "/data"}, "d1"); err != nil {
		t.Fatal(err)
	}
	_ = addVolume("shop", config.Volume{Name: "cache", Path: "/cache"})
	got := map[string]string{}
	for _, v := range listVolumes("shop") {
		got[v.Name] = v.Disk
	}
	if got["files"] != "/mnt/data" || got["cache"] != "" {
		t.Errorf("disks: %v", got)
	}
	if toml := projectToml(Source{App: "shop"}, nil, "net", listVolumes("shop")); !strings.Contains(toml, `disk = "/mnt/data"`) {
		t.Errorf("the deploy isn't told the disk:\n%s", toml)
	}
}
