package docker

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
)

func TestVolumeUsedWithNothingRunning(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	name := fmt.Sprintf("skiff-used-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", "-f", name).Run() })
	if out, err := exec.Command("docker", "run", "--rm", "-v", name+":/v", VolumeTool, "sh", "-c", "head -c 4194304 /dev/urandom > /v/f").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if n := Local().VolumeUsed(name); n < 4<<20 || n > 8<<20 {
		t.Fatalf("VolumeUsed = %d, want about 4 MB", n)
	}
}
