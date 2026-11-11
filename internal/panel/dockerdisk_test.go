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
	if got := p.volumeSizeNow(name); got < 4<<20 {
		t.Errorf("size = %d, want about 4 MB", got)
	}
}

func TestStorageDirIsAskedAgainUntilKnown(t *testing.T) {
	var s storageDir
	calls, answer := 0, ""
	ask := func() string { calls++; return answer }
	now := time.Now()
	if s.get(ask, now) != "" || s.get(ask, now.Add(10*time.Second)) != "" || calls != 1 {
		t.Fatalf("asked %d times within a minute", calls)
	}
	answer = "/var/lib/docker"
	if got := s.get(ask, now.Add(time.Minute)); got != answer || calls != 2 {
		t.Fatalf("got %q after %d asks", got, calls)
	}
	s.get(ask, now.Add(time.Hour))
	if calls != 2 {
		t.Errorf("asked again once known: %d", calls)
	}
}
