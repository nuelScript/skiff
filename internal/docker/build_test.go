package docker

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildPassesArgsAndTarget(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	dockerfile := "FROM " + VolumeTool + " AS runtime\nARG MODE=dev\nRUN echo \"$MODE\" > /mode\nFROM runtime AS last\nRUN echo last > /mode\n"
	tag := "skiff-build-test:" + strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", "-f", tag).Run() })
	if err := Local().BuildFromDockerfile(context.Background(), tag, dockerfile, dir, false, BuildOptions{Args: map[string]string{"MODE": "prod"}, Target: "runtime"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("docker", "run", "--rm", tag, "cat", "/mode").CombinedOutput()
	if got := strings.TrimSpace(string(out)); err != nil || got != "prod" {
		t.Errorf("/mode = %q (%v), want prod from the runtime stage", got, err)
	}
}

// TestRunOnceStopsAtItsDeadline checks a one-off run is removed when it times out, rather than left running after the client gives up.
func TestRunOnceStopsAtItsDeadline(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	marker := "skiff-deadline-" + strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "")
	_, err := Local().RunOnce(ctx, VolumeTool, "sleep 60 # "+marker, Settings{})
	if err == nil {
		t.Fatal("a run past its deadline should fail")
	}
	time.Sleep(2 * time.Second)
	out, _ := exec.Command("docker", "ps", "-a", "--no-trunc", "--format", "{{.Command}}").Output()
	if strings.Contains(string(out), marker) {
		t.Error("the container kept running after its deadline")
	}
}
