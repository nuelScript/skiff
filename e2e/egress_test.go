//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
		t.Cleanup(func() { _ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+")").Run() })
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

	deploy("e2e-shut", "egress = \"list\"\negress_allow = [\"1.1.1.1\"]\n")
	eventually(t, 30*time.Second, func() bool { return reaches("e2e-shut", "1.1.1.1") })
	if reaches("e2e-shut", "1.0.0.1") {
		t.Error("a host off the list should stay out of reach")
	}
}
