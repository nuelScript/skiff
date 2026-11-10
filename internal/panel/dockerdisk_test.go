package panel

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

func TestDiskUsageAtAFolder(t *testing.T) {
	if d := diskUsageAt(t.TempDir()); d.Total <= 0 || d.Used <= 0 {
		t.Errorf("a real folder: %+v", d)
	}
	if d := diskUsageAt("/no/such/folder"); d.Total != 0 {
		t.Errorf("a missing folder: %+v", d)
	}
}

// A volume kept on an added disk reads its real size with nothing mounting it, so its budget and backup room are right.
func TestAStoppedDiskVolumeHasItsSize(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	folder, _ := filepath.Abs(t.TempDir())
	if err := os.WriteFile(filepath.Join(folder, "data"), bytes.Repeat([]byte{7}, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("skiff-size-test-%d", time.Now().UnixNano())
	p := &Panel{eng: docker.Local()}
	if err := p.eng.CreateVolume(name, folder, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", "-f", name).Run() })
	volSizes.Lock()
	volSizes.sizes = nil
	volSizes.Unlock()
	if got := p.volumeSize(name); got < 4<<20 {
		t.Errorf("size = %d, want about 4 MB", got)
	}
}
