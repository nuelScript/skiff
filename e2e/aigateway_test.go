//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// aiClient calls the gateway from inside the app's container, the way an SDK would.
const aiClient = `import json, os, urllib.request
print("token-in-env", os.environ["SKIFF_AI_TOKEN"].startswith("skai_"))
print("key-in-env", any("sk-real-provider-key" in v for v in os.environ.values()))
req = urllib.request.Request(os.environ["SKIFF_AI_URL"] + "/fake/v1/chat/completions",
    data=b'{"model":"m"}', headers={"Content-Type": "application/json", "Authorization": "Bearer " + os.environ["SKIFF_AI_TOKEN"]})
print("answer", json.load(urllib.request.urlopen(req, timeout=10))["choices"][0]["message"]["content"])
`

// TestAIGatewayFromAContainer gives an app AI access and checks a container reaches the provider through Skiff, with the real key added on the way and never in the app.
func TestAIGatewayFromAContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	var sawKey atomic.Value
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey.Store(r.Header.Get("Authorization") + " " + r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"hello-from-the-model"}}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`)
	}))
	defer provider.Close()

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

	const app = "e2e-ai"
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+") ; docker rmi $(docker images -q skiff-"+app+")").Run()
	})
	dir := filepath.Join(root, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"client.py":  aiClient,
		"Dockerfile": "FROM python:3.12-alpine\nCOPY client.py /\nCMD [\"python\", \"-m\", \"http.server\", \"8080\"]\n",
		"skiff.toml": "name = \"" + app + "\"\n\n[build]\nport = 8080\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(t, dir, env, bin, "deploy")

	post(t, c, base+"/api/ai/providers", fmt.Sprintf(`{"name":"fake","kind":"custom","baseUrl":%q,"key":"sk-real-provider-key"}`, provider.URL))
	// The panel adopts the CLI app on its next pass; then it can be given access.
	eventually(t, 90*time.Second, func() bool {
		return putStatus(t, c, base+"/api/project/ai?app="+app, `{"enabled":true,"budget":1000}`) == http.StatusNoContent
	})

	var token struct{ Token string }
	postJSON(t, c, base+"/api/tokens", `{"name":"ci"}`, &token)
	api := func(method, path, body string, into any) {
		t.Helper()
		req, _ := http.NewRequest(method, base+"/api/v1/apps/"+app+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		_ = json.NewDecoder(resp.Body).Decode(into)
	}
	var started struct{ ID int64 }
	api(http.MethodPost, "/runs", `{"command":"python /client.py"}`, &started)
	var finished struct {
		Status, Output string
	}
	eventually(t, 2*time.Minute, func() bool {
		api(http.MethodGet, fmt.Sprintf("/runs/%d", started.ID), "", &finished)
		return finished.Status != "" && finished.Status != "running"
	})
	for _, want := range []string{"token-in-env True", "key-in-env False", "answer hello-from-the-model"} {
		if !strings.Contains(finished.Output, want) {
			t.Errorf("the run's output lacks %q:\n%s", want, finished.Output)
		}
	}
	if got, _ := sawKey.Load().(string); got != "Bearer sk-real-provider-key /v1/chat/completions" {
		t.Errorf("the provider got %q", got)
	}
	var usage struct {
		Used struct{ Input, Output, Requests int }
	}
	resp, err := c.Get(base + "/api/project/ai?app=" + app)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&usage)
	if usage.Used.Input != 11 || usage.Used.Output != 7 || usage.Used.Requests != 1 {
		t.Errorf("usage this month: %+v", usage.Used)
	}
}
