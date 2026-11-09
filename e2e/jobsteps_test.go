//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestJobSteps runs a job whose skiff.toml makes it a pipeline: a failed step stops what comes after it, a step beside it still runs, and the run keeps each step's outcome.
func TestJobSteps(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	protectExisting(t, home)
	env := []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup"}
	panel := freeAddr(t)
	start(t, env, bin, "panel", "--addr", panel, "--domain", "localhost")
	base := "http://" + panel
	waitHTTP(t, base+"/api/me")
	c := client(t)
	post(t, c, base+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	const app = "e2e-steps"
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+") ; docker rmi $(docker images -q skiff-"+app+")").Run()
	})
	dir := filepath.Join(root, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := `name = "` + app + `"

[build]
image = "python:3.12-alpine"
port = 8080

[deploy]
command = ["python", "-m", "http.server", "8080"]

[[jobs]]
name = "nightly"
schedule = "0 3 * * *"

[[jobs.steps]]
name = "extract"
command = "echo extract-ran"

[[jobs.steps]]
name = "transform"
command = "echo transform-ran; exit 3"
after = ["extract"]

[[jobs.steps]]
name = "load"
command = "echo load-ran"
after = ["transform"]

[[jobs.steps]]
name = "report"
command = "echo report-ran as $SKIFF_STEP"
after = ["extract"]
`
	if err := os.WriteFile(filepath.Join(dir, "skiff.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, env, bin, "deploy")

	var jobs []struct {
		ID    string
		Steps []struct{ Name string }
	}
	eventually(t, 90*time.Second, func() bool {
		resp, err := c.Get(base + "/api/jobs?app=" + app)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&jobs) == nil && len(jobs) == 1 && len(jobs[0].Steps) == 4
	})
	resp, err := c.Post(base+"/api/jobs/run?id="+jobs[0].ID, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var runs []struct {
		Status   string
		ExitCode int
		Output   string
		Steps    []struct{ Name, Status string }
	}
	getJSON(t, c, base+"/api/jobs/runs?id="+jobs[0].ID, &runs)
	if len(runs) != 1 {
		t.Fatalf("runs: %+v", runs)
	}
	r := runs[0]
	got := map[string]string{}
	for _, s := range r.Steps {
		got[s.Name] = s.Status
	}
	want := map[string]string{"extract": "ok", "transform": "failed", "load": "skipped", "report": "ok"}
	for name, status := range want {
		if got[name] != status {
			t.Errorf("step %s: %q, want %q (all: %v)", name, got[name], status, got)
		}
	}
	if r.Status != "failed" || r.ExitCode != 3 {
		t.Errorf("run: %s, exit %d", r.Status, r.ExitCode)
	}
	for _, s := range []string{"extract-ran", "transform-ran", "report-ran as report"} {
		if !strings.Contains(r.Output, s) {
			t.Errorf("output lacks %q:\n%s", s, r.Output)
		}
	}
	if strings.Contains(r.Output, "load-ran") {
		t.Errorf("a skipped step ran:\n%s", r.Output)
	}
}
