//go:build e2e

package e2e

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const udpApp = `import socket
VERSION = open("/version").read().strip()
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(("0.0.0.0", int(open("/port").read())))
while True:
    data, addr = s.recvfrom(2048)
    s.sendto(f"{VERSION} {data.decode()}".encode(), addr)
`

// TestUDPPortThroughTheRouter opens a public UDP port for an app and checks packets reach it through the router, before and after a deploy.
func TestUDPPortThroughTheRouter(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	protectExisting(t, home)
	env := []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup"}
	panel, routerAddr := freeAddr(t), freeAddr(t)
	start(t, env, bin, "panel", "--addr", panel, "--domain", "localhost")
	base := "http://" + panel
	waitHTTP(t, base+"/api/me")
	start(t, env, bin, "router", "--domain", "localhost", "--http-addr", routerAddr, "--panel", panel)
	c := client(t)
	post(t, c, base+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	const app = "e2e-udp"
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+") ; docker rmi $(docker images -q skiff-"+app+")").Run()
	})
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	pc.Close()

	dir := filepath.Join(root, "app")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write("server.py", udpApp)
	write("port", strconv.Itoa(port))
	// The app's HTTP side is only for its health check; the UDP port is what it's for.
	write("Dockerfile", "FROM python:3.12-alpine\nCOPY server.py version port /\nCMD [\"sh\", \"-c\", \"python -m http.server 8080 & python /server.py\"]\n")
	write("skiff.toml", fmt.Sprintf("name = %q\n\n[build]\nport = 8080\n\n[deploy]\nudp_port = %d\n", app, port))
	deploy := func(version string) {
		t.Helper()
		write("version", version)
		run(t, dir, env, bin, "deploy")
	}
	deploy("v1")

	eventually(t, 90*time.Second, func() bool {
		req, _ := http.NewRequest(http.MethodPut, base+"/api/project/udp-port?app="+app, strings.NewReader(fmt.Sprintf(`{"port":%d}`, port)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusNoContent
	})
	ask := func(msg string) string {
		conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return err.Error()
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(msg))
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 2048)
		n, err := conn.Read(buf)
		if err != nil {
			return err.Error()
		}
		return string(buf[:n])
	}
	eventually(t, 30*time.Second, func() bool { return ask("ping") == "v1 ping" })

	deploy("v2")
	eventually(t, 30*time.Second, func() bool { return strings.HasPrefix(ask("ping"), "v2 ") })
}
