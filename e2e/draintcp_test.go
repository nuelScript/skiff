//go:build e2e

package e2e

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// tcpApp sends its version and a count, once a second, for as many lines as the client asks for.
const tcpApp = `import socket, threading, time
VERSION = open("/version").read().strip()
def serve(conn):
    with conn:
        try:
            n = int(conn.makefile().readline())
        except Exception:
            return
        for i in range(n):
            conn.sendall(f"{VERSION} {i}\n".encode())
            time.sleep(1)
srv = socket.socket()
srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
srv.bind(("0.0.0.0", 8080))
srv.listen(64)
while True:
    conn, _ = srv.accept()
    threading.Thread(target=serve, args=(conn,), daemon=True).start()
`

// TestDeployLetsTCPConnectionsFinish replaces a version behind a public TCP port: the port keeps accepting throughout, a connection open during the deploy runs to its end on the old version, and new ones reach the new one.
func TestDeployLetsTCPConnectionsFinish(t *testing.T) {
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

	const app = "e2e-tcp"
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+") ; docker rmi $(docker images -q skiff-"+app+")").Run()
	})
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
	write("server.py", tcpApp)
	write("Dockerfile", "FROM python:3.12-alpine\nCOPY server.py version /\nCMD [\"python\", \"/server.py\"]\n")
	write("skiff.toml", "name = \""+app+"\"\n\n[build]\nport = 8080\n\n[deploy]\nhealth_mode = \"tcp\"\ndrain_timeout = 90\n")
	deploy := func(version string) {
		t.Helper()
		write("version", version)
		run(t, dir, env, bin, "deploy")
	}
	deploy("v1")

	// The panel adopts the CLI app on its next pass; then a team owner can open a public port for it.
	_, portText, _ := strings.Cut(freeAddr(t), ":")
	public, _ := strconv.Atoi(portText)
	eventually(t, 90*time.Second, func() bool {
		return putStatus(t, c, base+"/api/project/tcp-port?app="+app, fmt.Sprintf(`{"port":%d}`, public)) == http.StatusNoContent
	})
	addr := fmt.Sprintf("127.0.0.1:%d", public)
	ask := func(n int) ([]string, error) {
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Duration(n)*time.Second + 90*time.Second))
		fmt.Fprintf(conn, "%d\n", n)
		var lines []string
		for s := bufio.NewScanner(conn); s.Scan(); {
			lines = append(lines, s.Text())
		}
		return lines, nil
	}
	eventually(t, 30*time.Second, func() bool {
		got, err := ask(1)
		return err == nil && len(got) == 1 && got[0] == "v1 0"
	})

	// Keep connecting while the deploy runs.
	var failed, total atomic.Int64
	var firstFailure atomic.Value
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			total.Add(1)
			if got, err := ask(1); err != nil || len(got) != 1 {
				failed.Add(1)
				firstFailure.CompareAndSwap(nil, fmt.Sprintf("%v %v", got, err))
			}
		}
	}()
	long := make(chan []string, 1)
	go func() {
		got, _ := ask(25)
		long <- got
	}()
	time.Sleep(2 * time.Second)
	deploy("v2")
	time.Sleep(4 * time.Second)
	if got, err := ask(1); err != nil || len(got) != 1 || got[0] != "v2 0" {
		t.Errorf("a new connection after the deploy: %v %v", got, err)
	}
	got := <-long
	if len(got) != 25 || got[24] != "v1 24" {
		t.Errorf("the open connection was cut: %d lines, ending %v", len(got), got[max(0, len(got)-1):])
	}
	close(stop)
	<-stopped
	t.Logf("%d connections during the deploy, %d failed", total.Load(), failed.Load())
	if failed.Load() > 0 {
		t.Errorf("%d of %d connections failed during the deploy, first: %v", failed.Load(), total.Load(), firstFailure.Load())
	}
}

func putStatus(t *testing.T, c *http.Client, url, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}
