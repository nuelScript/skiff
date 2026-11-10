//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/hooksig"
)

// TestAgentFrameworks runs agents built on real frameworks as Skiff agents: each takes an event from its inbox, asks a model through the AI gateway, and finishes the event. The model is a fake that answers like OpenAI.
func TestAgentFrameworks(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// CrewAI reads its answer from "Final Answer:"; the others take the text as it is.
		io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"finish_reason":"stop",
			"message":{"role":"assistant","content":"Thought: I now know the final answer\nFinal Answer: proof-ok"}}],
			"usage":{"prompt_tokens":50,"completion_tokens":10,"total_tokens":60}}`)
	}))
	defer provider.Close()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	protectExisting(t, home)
	env := []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup", "SKIFF_ALLOW_PRIVATE_ENDPOINTS=1"}
	panel := freeAddr(t)
	start(t, env, bin, "panel", "--addr", panel, "--domain", "localhost")
	base := "http://" + panel
	waitHTTP(t, base+"/api/me")
	c := client(t)
	post(t, c, base+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)
	post(t, c, base+"/api/ai/providers", fmt.Sprintf(`{"name":"fake","kind":"custom","baseUrl":%q,"key":"sk-fake"}`, provider.URL))

	for _, fw := range []string{"otel", "langgraph", "crewai"} {
		t.Run(fw, func(t *testing.T) {
			app := "e2e-agent-" + fw
			t.Cleanup(func() {
				_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+app+") ; docker rmi $(docker images -q skiff-"+app+")").Run()
			})
			dir := t.TempDir()
			repo, push := gitServer(t, dir)
			for name, from := range map[string]string{"loop.py": "loop.py", "agent.py": fw + "/agent.py", "Dockerfile": fw + "/Dockerfile"} {
				b, err := os.ReadFile(filepath.Join("testdata", "agents", from))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "work", name), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			push("v1")
			stream(t, c, base+"/api/deploy?git="+url.QueryEscape(repo)+"&name="+app+"&port=3000&agent=1")

			var agent struct {
				Agent         bool
				WebhookSecret string
				Events        []struct {
					Source string
					Done   bool
				}
			}
			getJSON(t, c, base+"/api/project/agent?app="+app, &agent)
			if !agent.Agent || agent.WebhookSecret == "" {
				t.Fatalf("not set up as an agent: %+v", agent)
			}
			before := calls.Load()
			body := "Say hello in two words."
			req, _ := http.NewRequest(http.MethodPost, base+"/api/hooks/agents/"+app, strings.NewReader(body))
			hooksig.Set(req.Header, agent.WebhookSecret, []byte(body))
			resp, err := http.DefaultClient.Do(req)
			if err != nil || resp.StatusCode != http.StatusAccepted {
				t.Fatalf("webhook: %v %v", err, resp)
			}
			resp.Body.Close()

			finished := func() bool {
				getJSON(t, c, base+"/api/project/agent?app="+app, &agent)
				return len(agent.Events) == 1 && agent.Events[0].Done
			}
			deadline := time.Now().Add(5 * time.Minute)
			for !finished() {
				if time.Now().After(deadline) {
					logs, _ := exec.Command("sh", "-c", "docker logs --tail 60 $(docker ps -aq --filter label=skiff.app="+app+" | head -1) 2>&1").Output()
					t.Fatalf("the event wasn't finished: %+v\n%s", agent.Events, logs)
				}
				time.Sleep(3 * time.Second)
			}
			if calls.Load() == before {
				t.Error("the model was never called")
			}

			var ai struct {
				Used struct{ Input, Output, Requests int }
			}
			getJSON(t, c, base+"/api/project/ai?app="+app, &ai)
			if ai.Used.Requests < 1 || ai.Used.Input < 50 || ai.Used.Output < 10 {
				t.Errorf("usage through the gateway: %+v", ai.Used)
			}
			var traces struct {
				Traces []struct {
					Name       string
					Spans      int
					ModelCalls int
				}
			}
			getJSON(t, c, base+"/api/project/traces?app="+app, &traces)
			if len(traces.Traces) == 0 {
				t.Fatal("no runs recorded")
			}
			if fw == "otel" {
				// The agent's own step and the gateway's model call are one run.
				joined := false
				for _, tr := range traces.Traces {
					joined = joined || tr.Name == "handle event" && tr.Spans >= 2 && tr.ModelCalls >= 1
				}
				if !joined {
					raw, _ := json.Marshal(traces)
					t.Errorf("the agent's span and the model call aren't one run: %s", raw)
				}
			}
		})
	}
}
