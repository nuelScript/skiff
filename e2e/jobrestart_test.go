//go:build e2e

package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestJobRunsSurviveRestart kills the panel mid-run: a run still going is followed to its end, and one whose container died too is tried again and resumes from its checkpoint on a volume.
func TestJobRunsSurviveRestart(t *testing.T) {
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
	stop := startKillable(t, env, bin, "panel", "--addr", panel, "--domain", "localhost")
	base := "http://" + panel
	waitHTTP(t, base+"/api/me")
	c := client(t)
	post(t, c, base+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	const app = "e2e-jobrestart"
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+") $(docker ps -aq --filter label=skiff.job-run) ; docker rmi $(docker images -q skiff-"+app+") ; docker volume rm -f skiff-"+app+"_ck").Run()
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

[[volumes]]
name = "ck"
path = "/ck"

[[jobs]]
name = "long"
command = "for i in $(seq 1 25); do echo tick $i; sleep 1; done; echo finished"
webhook = true
timeout = "10m"

[[jobs]]
name = "resume"
command = "if [ -f /ck/started ]; then echo resumed-from-checkpoint; exit 0; fi; touch /ck/started; echo first-attempt; sleep 600"
webhook = true
retries = 1
timeout = "20m"
`
	if err := os.WriteFile(filepath.Join(dir, "skiff.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, env, bin, "deploy")

	type job struct {
		ID, Name, WebhookSecret string
	}
	var jobs []job
	eventually(t, time.Minute, func() bool {
		jobs = nil
		resp, err := c.Get(base + "/api/jobs?app=" + app)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&jobs) == nil && len(jobs) == 2
	})
	byName := map[string]job{}
	for _, j := range jobs {
		byName[j.Name] = j
	}
	type jobRun struct {
		ID      int64
		Trigger string
		Attempt int
		Status  string
		Output  string
	}
	runs := func(name string) []jobRun {
		var out []jobRun
		resp, err := c.Get(base + "/api/jobs/runs?id=" + byName[name].ID)
		if err == nil {
			defer resp.Body.Close()
			_ = json.NewDecoder(resp.Body).Decode(&out)
		}
		return out
	}
	for _, name := range []string{"long", "resume"} {
		j := byName[name]
		mac := hmac.New(sha256.New, []byte(j.WebhookSecret))
		mac.Write([]byte("{}"))
		req, _ := http.NewRequest(http.MethodPost, base+"/api/hooks/jobs/"+j.ID, strings.NewReader("{}"))
		req.Header.Set("X-Skiff-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("webhook for %s = %d", name, resp.StatusCode)
		}
	}
	container := func(run int64) string {
		out, _ := exec.Command("docker", "ps", "-q", "--filter", fmt.Sprintf("label=skiff.job-run=%d", run)).Output()
		return strings.TrimSpace(string(out))
	}
	var long, resume int64
	eventually(t, 2*time.Minute, func() bool {
		l, r := runs("long"), runs("resume")
		if len(l) == 0 || len(r) == 0 || l[0].Status != "running" || r[0].Status != "running" {
			return false
		}
		long, resume = l[0].ID, r[0].ID
		return container(long) != "" && container(resume) != ""
	})

	stop()
	// The second run's container goes too, as if the server had rebooted under it.
	if out, err := exec.Command("docker", "rm", "-f", container(resume)).CombinedOutput(); err != nil {
		t.Fatalf("removing the run's container: %v %s", err, out)
	}
	startKillable(t, env, bin, "panel", "--addr", panel, "--domain", "localhost")
	waitHTTP(t, base+"/api/me")

	eventually(t, 2*time.Minute, func() bool {
		l := runs("long")
		return len(l) > 0 && l[0].ID == long && l[0].Status == "ok"
	})
	l := runs("long")[0]
	if !strings.Contains(l.Output, "tick 1\n") || !strings.Contains(l.Output, "finished") || !strings.Contains(l.Output, "kept going") {
		t.Errorf("the followed run's output:\n%s", l.Output)
	}

	eventually(t, 2*time.Minute, func() bool {
		r := runs("resume")
		return len(r) >= 2 && r[0].Status == "ok"
	})
	r := runs("resume")
	if r[1].ID != resume || r[1].Status != "failed" || !strings.Contains(r[1].Output, "restarted") {
		t.Errorf("the cut-off run: %+v", r[1])
	}
	if r[0].Attempt != 2 || r[0].Trigger != "retry" || !strings.Contains(r[0].Output, "resumed-from-checkpoint") {
		t.Errorf("the retry: %+v", r[0])
	}
}

// startKillable starts a process and returns a kill -9 for it, as a crash or an upgrade would stop it.
func startKillable(t *testing.T, env []string, bin string, args ...string) func() {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	logf, _ := os.CreateTemp(t.TempDir(), args[0]+"-*.log")
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stop := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	t.Cleanup(func() {
		stop()
		if t.Failed() {
			b, _ := os.ReadFile(logf.Name())
			t.Logf("%s log:\n%s", args[0], b)
		}
	})
	return stop
}
