//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/egress"
)

// TestEgress limits where an app may connect and checks the server's rules hold it there, while an app beside it on the same network is untouched. It needs root and iptables, which the sandbox workflow gives it.
func TestEgress(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("needs Linux and root, to change the server's firewall")
	}
	if _, err := exec.LookPath("iptables-restore"); err != nil {
		t.Skip("needs iptables")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	protectExisting(t, home)
	env := []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup"}
	panel := freeAddr(t)
	start(t, env, bin, "panel", "--addr", panel, "--domain", "localhost")
	waitHTTP(t, "http://"+panel+"/api/me")

	deploy := func(app, egress string) {
		t.Helper()
		t.Cleanup(func() {
			_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+")").Run()
		})
		dir := filepath.Join(root, app)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		toml := "name = \"" + app + "\"\n\n[build]\nimage = \"python:3.12-alpine\"\nport = 8080\n\n[deploy]\ncommand = [\"python\", \"-m\", \"http.server\", \"8080\"]\n" + egress
		if err := os.WriteFile(filepath.Join(dir, "skiff.toml"), []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, env, bin, "deploy")
	}
	reaches := func(app, ip string) bool {
		out, _ := exec.Command("sh", "-c", "docker exec $(docker ps -q --filter label=skiff.app="+app+" | head -1) python -c \"import socket; socket.create_connection(('"+ip+"', 80), 4); print('yes')\"").CombinedOutput()
		return strings.Contains(string(out), "yes")
	}

	deploy("e2e-open", "")
	deploy("e2e-shut", "egress = \"private\"\n")
	eventually(t, 30*time.Second, func() bool { return !reaches("e2e-shut", "1.1.1.1") })
	if !reaches("e2e-open", "1.1.1.1") {
		t.Error("an app without limits on the same network should still reach the internet")
	}

	// A firewall reload that drops Skiff's rules is noticed and the limit comes back.
	if out, err := exec.Command("iptables", "-w", "-D", "DOCKER-USER", "-j", "SKIFF-EGRESS").CombinedOutput(); err != nil {
		t.Fatalf("dropping the jump: %v %s", err, out)
	}
	eventually(t, 75*time.Second, func() bool {
		return exec.Command("iptables", "-w", "-C", "DOCKER-USER", "-j", "SKIFF-EGRESS").Run() == nil && !reaches("e2e-shut", "1.1.1.1")
	})

	deploy("e2e-shut", "egress = \"list\"\negress_allow = [\"1.1.1.1\"]\n")
	eventually(t, 30*time.Second, func() bool { return reaches("e2e-shut", "1.1.1.1") })
	if reaches("e2e-shut", "1.0.0.1") {
		t.Error("a host off the list should stay out of reach")
	}

	// A sandbox of an app with a list runs on the sandbox network with the app's policy, and the same rules hold it.
	eng := docker.Local()
	if err := eng.EnsureSandboxNetwork(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eng.RemoveSandboxes("e2e-egress-sbx") })
	// Another team's sandbox on the same network, which must stay out of reach.
	other := "e2e-egress-neighbour"
	if out, err := exec.Command("docker", "run", "-d", "--name", other, "--network", docker.SandboxNetwork, "python:3.12-alpine", "python", "-m", "http.server", "8080").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", other).Run() })
	ipOut, _ := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", other).Output()
	neighbour := strings.TrimSpace(string(ipOut))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := eng.Sandbox(ctx, docker.SandboxSpec{
		App: "e2e-egress-sbx", Image: "python:3.12-alpine", Memory: "256m", CPU: "1", Internet: true,
		Network: docker.SandboxNetwork, Labels: map[string]string{egress.Label: egress.Policy{Mode: egress.List, Allow: []string{"1.1.1.1"}}.Encode()},
		// The rules follow a container's start, so give them a moment.
		Command: `sleep 5; for ip in 1.1.1.1 1.0.0.1; do python -c "import socket; socket.create_connection(('$ip', 80), 4); print('$ip yes')" 2>/dev/null || echo "$ip no"; done
python -c "import socket; socket.create_connection(('` + neighbour + `', 8080), 4); print('neighbour yes')" 2>/dev/null || echo "neighbour no"`,
	})
	if err != nil || !strings.Contains(res.Output, "1.1.1.1 yes") || !strings.Contains(res.Output, "1.0.0.1 no") || !strings.Contains(res.Output, "neighbour no") {
		t.Errorf("sandbox with a list: %v\n%s", err, res.Output)
	}
}
