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
