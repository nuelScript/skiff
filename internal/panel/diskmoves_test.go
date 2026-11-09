package panel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
