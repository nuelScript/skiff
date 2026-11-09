//go:build e2e

package e2e

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestTomlWorkersAndJobs deploys an app whose skiff.toml lists a worker and a job, and checks the CLI runs the worker on each new version while the panel shows both and runs the job.
func TestTomlWorkersAndJobs(t *testing.T) {
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

	const app = "e2e-toml"
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+") $(docker ps -aq --filter label=skiff.worker.app="+app+") ; docker rmi $(docker images -q skiff-"+app+")").Run()
	})
	dir := filepath.Join(root, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	deploy := func(work string) {
		t.Helper()
		toml := "name = \"" + app + "\"\n\n[build]\nimage = \"python:3.12-alpine\"\nport = 8080\n\n[deploy]\ncommand = [\"python\", \"-m\", \"http.server\", \"8080\"]\n\n[resources]\nmemory = \"64m\"\n\n" + work
		if err := os.WriteFile(filepath.Join(dir, "skiff.toml"), []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, env, bin, "deploy")
	}
	workers := func() []string {
		out, _ := exec.Command("docker", "ps", "-q", "--filter", "label=skiff.worker.app="+app).Output()
		return strings.Fields(string(out))
	}
	// Until the panel adopts the app, these answer forbidden, which reads as empty.
	fetch := func(path string, into any) {
		resp, err := c.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			_ = json.NewDecoder(resp.Body).Decode(into)
		}
	}
	type job struct{ ID, Name, Schedule string }
	jobs := func() []job {
		var out []job
		fetch("/api/jobs?app="+app, &out)
		return out
	}
	type worker struct {
		Name     string
		Replicas int
		Running  int
	}
	listed := func() []worker {
		var out []worker
		fetch("/api/workers?app="+app, &out)
		return out
	}

	deploy("[[workers]]\nname = \"queue\"\ncommand = \"sleep 100000\"\nreplicas = 2\n\n[[jobs]]\nname = \"report\"\nschedule = \"0 3 * * *\"\ncommand = \"echo job-ran; cat /sys/fs/cgroup/memory.max\"\n")
	if got := workers(); len(got) != 2 {
		t.Fatalf("%d workers running after the deploy, want 2", len(got))
	}
	appImage, _ := exec.Command("sh", "-c", "docker inspect -f '{{.Image}}' $(docker ps -q --filter label=skiff.app="+app+" | head -1)").Output()
	workerImage, _ := exec.Command("docker", "inspect", "-f", "{{.Image}}", workers()[0]).Output()
	if strings.TrimSpace(string(appImage)) != strings.TrimSpace(string(workerImage)) {
		t.Errorf("the worker runs %s, the app %s", workerImage, appImage)
	}

	// The panel picks the CLI app up on its next pass.
	eventually(t, 90*time.Second, func() bool {
		l := listed()
		return len(l) == 1 && l[0].Name == "queue" && l[0].Replicas == 2 && l[0].Running == 2 && len(jobs()) == 1
	})
	first := jobs()[0]
	var ran struct {
		OK     bool
		Output string
	}
	resp, err := c.Post(base+"/api/jobs/run?id="+first.ID, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&ran)
	resp.Body.Close()
	if !ran.OK || !strings.Contains(ran.Output, "job-ran") {
		t.Errorf("running the job: %+v", ran)
	}
	// The job runs within the app's own memory limit from skiff.toml, not the server-wide cap.
	if !strings.Contains(ran.Output, "67108864") {
		t.Errorf("the job didn't get the app's 64m memory limit: %q", ran.Output)
	}
	type jobRun struct {
		Trigger, Status, Output string
		ExitCode                int
	}
	runs := func(id string) []jobRun {
		var out []jobRun
		fetch("/api/jobs/runs?id="+id, &out)
		return out
	}
	if r := runs(first.ID); len(r) != 1 || r[0].Status != "ok" || r[0].Trigger != "manual" || !strings.Contains(r[0].Output, "job-ran") {
		t.Errorf("the run should be in the job's history: %+v", r)
	}
	resp, err = c.Post(base+"/api/workers?app="+app, "application/json", strings.NewReader(`{"name":"other","command":"sleep 1","replicas":1}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("adding a worker to an app whose skiff.toml owns them: %d", resp.StatusCode)
	}

	// A changed schedule keeps the job, and when it last ran.
	deploy("[[workers]]\nname = \"queue\"\ncommand = \"sleep 100000\"\n\n[[jobs]]\nname = \"report\"\nschedule = \"0 4 * * *\"\ncommand = \"echo job-ran; exit 4\"\ntimeout = \"5m\"\nretries = 2\n")
	if got := workers(); len(got) != 1 {
		t.Errorf("%d workers after lowering replicas, want 1", len(got))
	}
	eventually(t, 90*time.Second, func() bool {
		j := jobs()
		return len(j) == 1 && j[0].Schedule == "0 4 * * *"
	})
	if j := jobs(); j[0].ID != first.ID {
		t.Errorf("the job was replaced, not changed: %s became %s", first.ID, j[0].ID)
	}
	var settings []struct{ Timeout, Retries int }
	fetch("/api/jobs?app="+app, &settings)
	if len(settings) != 1 || settings[0].Timeout != 300 || settings[0].Retries != 2 {
		t.Errorf("timeout and retries from skiff.toml: %+v", settings)
	}
	// Running it by hand tries once, and the history keeps the exit code.
	resp, err = c.Post(base+"/api/jobs/run?id="+first.ID, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if r := runs(first.ID); len(r) != 2 || r[0].Status != "failed" || r[0].ExitCode != 4 {
		t.Errorf("a failed run should keep its exit code: %+v", r)
	}

	// A one-off run through the API streams its output, keeps its exit code, and can be stopped.
	var token struct{ Token string }
	postJSON(t, c, base+"/api/tokens", `{"name":"ci"}`, &token)
	api := func(method, path, body string, into any) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(method, base+"/api/v1/apps/"+app+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if into != nil {
			_ = json.Unmarshal(raw, into)
		}
		return resp.StatusCode, string(raw)
	}
	type oneOff struct {
		ID             int64
		Status, Output string
		ExitCode       int
	}
	var started oneOff
	if code, msg := api(http.MethodPost, "/runs", `{"command":"echo out-1; sleep 2; echo out-2; exit 7"}`, &started); code != http.StatusAccepted || started.Status != "running" {
		t.Fatalf("starting a run: %d %s", code, msg)
	}
	runPath := "/runs/" + strconv.FormatInt(started.ID, 10)
	if _, streamed := api(http.MethodGet, runPath+"/logs", "", nil); !strings.Contains(streamed, "out-1") || !strings.Contains(streamed, "out-2") {
		t.Errorf("streamed output: %q", streamed)
	}
	var finished oneOff
	api(http.MethodGet, runPath, "", &finished)
	if finished.Status != "failed" || finished.ExitCode != 7 || !strings.Contains(finished.Output, "out-2") {
		t.Errorf("a finished run: %+v", finished)
	}
	var long oneOff
	api(http.MethodPost, "/runs", `{"command":"sleep 120"}`, &long)
	if code, msg := api(http.MethodDelete, "/runs/"+strconv.FormatInt(long.ID, 10), "", &long); code != http.StatusOK || long.Status != "failed" || !strings.Contains(long.Output, "Stopped by request") {
		t.Errorf("stopping a run: %d %s", code, msg)
	}
	if out, _ := exec.Command("docker", "ps", "-q", "--filter", "name=skiff-run-").Output(); strings.TrimSpace(string(out)) != "" {
		t.Errorf("a stopped run's container is still there: %s", out)
	}

	// A job with a webhook runs when a signed request arrives, and gets its body.
	deploy("[[jobs]]\nname = \"hooked\"\nwebhook = true\ncommand = \"echo got:$SKIFF_WEBHOOK_BODY\"\n")
	var hooked struct{ ID, Webhook, WebhookSecret string }
	eventually(t, 90*time.Second, func() bool {
		var list []struct{ ID, Name, Webhook, WebhookSecret string }
		fetch("/api/jobs?app="+app, &list)
		for _, j := range list {
			if j.Name == "hooked" && j.WebhookSecret != "" {
				hooked.ID, hooked.Webhook, hooked.WebhookSecret = j.ID, j.Webhook, j.WebhookSecret
				return true
			}
		}
		return false
	})
	body := `{"ref":"main"}`
	mac := hmac.New(sha256.New, []byte(hooked.WebhookSecret))
	mac.Write([]byte(body))
	req, _ := http.NewRequest(http.MethodPost, base+"/api/hooks/jobs/"+hooked.ID, strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("calling the webhook: %d", resp.StatusCode)
	}
	eventually(t, 2*time.Minute, func() bool {
		r := runs(hooked.ID)
		return len(r) == 1 && r[0].Status == "ok" && r[0].Trigger == "webhook" && strings.Contains(r[0].Output, `got:{"ref":"main"}`)
	})

	deploy("")
	if got := workers(); len(got) != 0 {
		t.Errorf("%d workers still running after skiff.toml dropped them", len(got))
	}
	eventually(t, 90*time.Second, func() bool { return len(listed()) == 0 && len(jobs()) == 0 })
}
