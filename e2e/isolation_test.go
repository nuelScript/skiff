//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStrongIsolation deploys an app under gVisor and compares how long the same work takes with and without it. It needs gVisor's runsc registered with Docker, which the sandbox workflow installs.
func TestStrongIsolation(t *testing.T) {
	if out, err := exec.Command("docker", "info", "--format", "{{json .Runtimes}}").Output(); err != nil || !strings.Contains(string(out), `"runsc"`) {
		t.Skip("gVisor's runsc isn't registered with Docker")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	protectExisting(t, home)
	env := []string{"HOME=" + home}

	const app = "e2e-sandbox"
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+")").Run()
	})
	dir := filepath.Join(root, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := "name = \"" + app + "\"\n\n[build]\nimage = \"python:3.12-alpine\"\nport = 8080\n\n[deploy]\ncommand = [\"python\", \"-m\", \"http.server\", \"8080\"]\nisolation = \"strong\"\n"
	if err := os.WriteFile(filepath.Join(dir, "skiff.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, env, bin, "deploy")
	out, _ := exec.Command("sh", "-c", "docker inspect -f '{{.HostConfig.Runtime}}' $(docker ps -q --filter label=skiff.app="+app+" | head -1)").Output()
	if got := strings.TrimSpace(string(out)); got != "runsc" {
		t.Fatalf("the app runs under %q, want runsc", got)
	}

	// The cost: the same work, once under each runtime.
	work := "python -c \"import hashlib,os; [hashlib.sha256(os.urandom(65536)).digest() for _ in range(20000)]\"; for i in $(seq 1 2000); do echo x > /tmp/f$i; done; rm -f /tmp/f*"
	took := func(extra ...string) time.Duration {
		args := append([]string{"run", "--rm"}, extra...)
		args = append(args, "python:3.12-alpine", "sh", "-c", work)
		start := time.Now()
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return time.Since(start)
	}
	standard, strong := took(), took("--runtime", "runsc")
	t.Logf("the same work: %s standard, %s under gVisor (%.1fx)", standard.Round(time.Millisecond), strong.Round(time.Millisecond), float64(strong)/float64(standard))
}
