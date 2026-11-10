package panel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/docker"
)

func TestARestartSettlesAnInterruptedMove(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	openTestDB(t)
	p := &Panel{eng: docker.Local()}
	read := func(volume string) string {
		out, _ := exec.Command("docker", "run", "--rm", "-v", volume+":/v", docker.VolumeTool, "cat", "/v/f").Output()
		return strings.TrimSpace(string(out))
	}
	newVolume := func(name string) {
		t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", "-f", name).Run() })
		if out, err := exec.Command("docker", "run", "--rm", "-v", name+":/v", docker.VolumeTool, "sh", "-c", "echo kept > /v/f").CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	disk, _ := filepath.Abs(t.TempDir())

	// Cut off while copying: the copy so far is dropped and the volume stays in Docker's storage.
	vol := "skiff-move-test-a"
	newVolume(vol)
	dest := filepath.Join(disk, vol)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dest, "half"), []byte("x"), 0o644)
	if err := saveMove(moveRecord{Volume: vol, Dest: dest, Phase: "copying", Labels: p.eng.VolumeLabels(vol)}); err != nil {
		t.Fatal(err)
	}
	p.resumeMoves()
	if f := p.eng.VolumeFolder(vol); f != "" || read(vol) != "kept" {
		t.Fatalf("after an interrupted copy the volume is in %q with %q", f, read(vol))
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("the part-made copy was left on the disk: %v", entries)
	}
	if moveStatus(vol).Error == "" {
		t.Error("the interrupted move wasn't reported")
	}

	// Cut off after the copy was checked and the old volume removed: the move is finished on the disk.
	vol = "skiff-move-test-b"
	dest = filepath.Join(disk, vol)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dest, "f"), []byte("kept\n"), 0o644)
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", "-f", vol).Run() })
	if err := saveMove(moveRecord{Volume: vol, Dest: dest, Phase: "copied"}); err != nil {
		t.Fatal(err)
	}
	p.resumeMoves()
	if f := p.eng.VolumeFolder(vol); f != dest || read(vol) != "kept" {
		t.Fatalf("after a checked copy the volume is in %q with %q", f, read(vol))
	}
	if len(listMoveRecords()) != 0 {
		t.Errorf("moves left recorded: %+v", listMoveRecords())
	}
}

// While an app's volume or one it borrows is moving, its containers wait, so none of them makes an empty volume in its place.
func TestStartsWaitForAMove(t *testing.T) {
	openTestDB(t)
	_ = addVolume("shop", config.Volume{Name: "files", Path: "/data"})
	_ = addVolume("worker", config.Volume{Name: "files", Path: "/data", Owner: "shop"})
	name := config.VolumeName("shop", "files")
	if !startMove(name, "/mnt/data") {
		t.Fatal("move didn't start")
	}
	defer endMove(name, nil)
	for _, app := range []string{"shop", "worker"} {
		if movingVolume(app) != name {
			t.Errorf("%s doesn't see its volume moving", app)
		}
	}
	defer func(d time.Duration) { moveWaitEvery = d }(moveWaitEvery)
	moveWaitEvery = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if waitForMoves(ctx, "worker") == nil {
		t.Error("a start went ahead during the move")
	}
	if _, err := (&Panel{eng: docker.Local()}).runVolumeBackup(Source{App: "shop"}, "files", "manual"); err == nil || !strings.Contains(err.Error(), "being moved") {
		t.Errorf("a backup during the move: %v", err)
	}
	go func() { time.Sleep(50 * time.Millisecond); endMove(name, nil) }()
	if err := waitForMoves(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
}
