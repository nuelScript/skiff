//go:build e2e

package e2e

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

// TestSandboxRunsCodeAndHandsBackFiles runs code an agent wrote in a throwaway container: files go in, files come out, the network is off unless asked for, and nothing is left behind.
func TestSandboxRunsCodeAndHandsBackFiles(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	eng := docker.Local()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Cleanup(func() { eng.RemoveSandboxes("e2e-sbx") })
	spec := docker.SandboxSpec{
		App: "e2e-sbx", Image: "python:3.12-alpine", Memory: "256m", CPU: "1",
		Command: "python src/main.py",
		Files: map[string][]byte{
			"src/main.py": []byte(`import os, socket
print("input:", open("data/in.txt").read().strip())
os.makedirs("out", exist_ok=True)
open("out/result.txt", "w").write("42\n")
try:
    socket.create_connection(("1.1.1.1", 53), timeout=3)
    print("network: on")
except OSError:
    print("network: off")
`),
			"data/in.txt": []byte("hello sandbox\n"),
		},
	}
	res, err := eng.Sandbox(ctx, spec)
	if err != nil {
		t.Fatalf("sandbox: %v\n%s", err, res.Output)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Output, "input: hello sandbox") || !strings.Contains(res.Output, "network: off") {
		t.Errorf("exit %d, output:\n%s", res.ExitCode, res.Output)
	}
	if string(res.Files["out/result.txt"]) != "42\n" || string(res.Files["src/main.py"]) == "" {
		t.Errorf("files back: %v", keys(res.Files))
	}

	// A failing command keeps its exit code, and the internet can be asked for.
	spec.Files, spec.Internet = nil, true
	spec.Command = `python -c "import socket; socket.create_connection(('1.1.1.1', 53), timeout=5); print('network: on')"; exit 3`
	res, err = eng.Sandbox(ctx, spec)
	if err != nil || res.ExitCode != 3 || !strings.Contains(res.Output, "network: on") {
		t.Errorf("with internet: %v exit %d\n%s", err, res.ExitCode, res.Output)
	}

	// Running out of time stops it, and stopping an app's sandboxes leaves none behind.
	short, cancelShort := context.WithTimeout(ctx, 3*time.Second)
	defer cancelShort()
	spec.Command = "sleep 60"
	if _, err := eng.Sandbox(short, spec); err == nil {
		t.Error("a sandbox past its time should fail")
	}
	for _, kind := range [][]string{{"ps", "-aq"}, {"volume", "ls", "-q"}} {
		if out, _ := exec.Command("docker", append(kind, "--filter", "label="+docker.SandboxLabel+"=e2e-sbx")...).Output(); strings.TrimSpace(string(out)) != "" {
			t.Errorf("sandboxes left behind (%s): %s", kind[0], out)
		}
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestBrowserSandbox drives Chromium through Playwright in a sandbox and gets its screenshot back.
func TestBrowserSandbox(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	eng := docker.Local()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	t.Cleanup(func() { eng.RemoveSandboxes("e2e-browser") })
	if err := eng.EnsureBrowserImage(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Sandbox(ctx, docker.SandboxSpec{
		App: "e2e-browser", Image: docker.BrowserImage, Memory: "2g", CPU: "1", ShmSize: "1g",
		Command: "python shot.py",
		Files: map[string][]byte{"shot.py": []byte(`from playwright.sync_api import sync_playwright
with sync_playwright() as p:
    browser = p.chromium.launch()
    page = browser.new_page()
    page.set_content("<h1 id=t>hello from a sandbox</h1>")
    print("title text:", page.inner_text("#t"))
    page.screenshot(path="shot.png")
    browser.close()
`)},
	})
	if err != nil {
		t.Fatalf("browser sandbox: %v\n%s", err, res.Output)
	}
	png := res.Files["shot.png"]
	if res.ExitCode != 0 || !strings.Contains(res.Output, "title text: hello from a sandbox") || len(png) < 100 || string(png[1:4]) != "PNG" {
		t.Errorf("exit %d, %d bytes of screenshot, output:\n%s", res.ExitCode, len(png), res.Output)
	}
}

// TestSandboxUnderGVisor runs a sandbox with gVisor's runtime, as Skiff does when the server has it: files still go in and out, and the code sees gVisor's kernel.
func TestSandboxUnderGVisor(t *testing.T) {
	if out, err := exec.Command("docker", "info", "--format", "{{json .Runtimes}}").Output(); err != nil || !strings.Contains(string(out), `"runsc"`) {
		t.Skip("gVisor's runsc isn't registered with Docker")
	}
	eng := docker.Local()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t.Cleanup(func() { eng.RemoveSandboxes("e2e-gvisor") })
	res, err := eng.Sandbox(ctx, docker.SandboxSpec{
		App: "e2e-gvisor", Image: "python:3.12-alpine", Memory: "256m", CPU: "1", Runtime: docker.SandboxRuntime,
		Command: "dmesg | head -3; cat in.txt > out.txt",
		Files:   map[string][]byte{"in.txt": []byte("through gVisor\n")},
	})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("%v exit %d\n%s", err, res.ExitCode, res.Output)
	}
	if !strings.Contains(res.Output, "gVisor") || string(res.Files["out.txt"]) != "through gVisor\n" {
		t.Errorf("output:\n%s\nfiles: %v", res.Output, keys(res.Files))
	}
}
