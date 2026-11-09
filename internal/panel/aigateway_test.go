package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/seal"
)

// fakeProvider answers like a model API and records what reached it.
type fakeProvider struct {
	auth, apiKey, path string
	body               map[string]any
}

func (f *fakeProvider) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.auth, f.apiKey, f.path = r.Header.Get("Authorization"), r.Header.Get("x-api-key"), r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		f.body = nil
		_ = json.Unmarshal(raw, &f.body)
		if f.body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":30,\"output_tokens\":1}}}\n\n")
			w.(http.Flusher).Flush()
			io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hi\"}}\n\n")
			io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":12}}\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":100,"completion_tokens":20}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func addProvider(t *testing.T, team, name, kind, base, key string) {
	t.Helper()
	sealed, err := seal.Seal(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO ai_providers(team, name, kind, base_url, api_key, created) VALUES(?,?,?,?,?,1)`, team, name, kind, base, sealed); err != nil {
		t.Fatal(err)
	}
}

func callGateway(token, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	aiGateway(w, r)
	return w
}

func TestAIGatewayKeepsTheKeyAndCountsTokens(t *testing.T) {
	openTestDB(t)
	f := &fakeProvider{}
	srv := f.serve(t)
	addProvider(t, "t1", "openai", "openai", srv.URL, "sk-real-key")
	addProvider(t, "t1", "claude", "anthropic", srv.URL, "sk-ant-real")
	addProvider(t, "t2", "other", "openai", srv.URL, "sk-other-team")
	if err := setAIApp(Source{App: "agent", Team: "t1"}, true, 0); err != nil {
		t.Fatal(err)
	}
	a, _ := aiAppFor("agent")

	if w := callGateway("", "/openai/v1/chat/completions", `{}`); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", w.Code)
	}
	if w := callGateway("skai_wrong", "/openai/v1/chat/completions", `{}`); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", w.Code)
	}
	if w := callGateway(a.token, "/other/v1/chat/completions", `{}`); w.Code != http.StatusNotFound {
		t.Errorf("another team's provider: %d", w.Code)
	}

	w := callGateway(a.token, "/openai/v1/chat/completions", `{"model":"m"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"hi"`) {
		t.Fatalf("call: %d %s", w.Code, w.Body)
	}
	if f.auth != "Bearer sk-real-key" || f.path != "/v1/chat/completions" {
		t.Errorf("provider got auth %q path %q", f.auth, f.path)
	}
	if got := aiUsed("agent", time.Now()); got != (aiTotals{Input: 100, Output: 20, Requests: 1}) {
		t.Errorf("after a JSON call: %+v", got)
	}

	// An OpenAI stream is asked to report its usage; Anthropic's carries it anyway, as x-api-key.
	callGateway(a.token, "/openai/v1/chat/completions", `{"stream":true}`)
	if opts, _ := f.body["stream_options"].(map[string]any); opts["include_usage"] != true {
		t.Errorf("stream options sent: %v", f.body["stream_options"])
	}
	r := httptest.NewRequest(http.MethodPost, "/claude/v1/messages", strings.NewReader(`{"stream":true}`))
	r.Header.Set("x-api-key", a.token)
	r.Header.Set("Content-Type", "application/json")
	aiGateway(httptest.NewRecorder(), r)
	if f.apiKey != "sk-ant-real" || f.auth != "" {
		t.Errorf("anthropic got key %q auth %q", f.apiKey, f.auth)
	}
	if _, ok := f.body["stream_options"]; ok {
		t.Error("anthropic request was changed")
	}
	if got := aiUsed("agent", time.Now()); got != (aiTotals{Input: 160, Output: 44, Requests: 3}) {
		t.Errorf("after two streams: %+v", got)
	}
}

func TestAIGatewayStopsAtTheBudget(t *testing.T) {
	openTestDB(t)
	f := &fakeProvider{}
	addProvider(t, "t1", "openai", "openai", f.serve(t).URL, "sk-real-key")
	if err := setAIApp(Source{App: "agent", Team: "t1"}, true, 200); err != nil {
		t.Fatal(err)
	}
	a, _ := aiAppFor("agent")
	for i, want := range []int{200, 200, 429} {
		if w := callGateway(a.token, "/openai/v1/chat/completions", `{}`); w.Code != want {
			t.Errorf("call %d: %d, want %d", i+1, w.Code, want)
		}
	}
	// Raising the budget keeps the token.
	if err := setAIApp(Source{App: "agent", Team: "t1"}, true, 1000); err != nil {
		t.Fatal(err)
	}
	if again, _ := aiAppFor("agent"); again.token != a.token {
		t.Error("the token changed with the budget")
	}
	if w := callGateway(a.token, "/openai/v1/chat/completions", `{}`); w.Code != http.StatusOK {
		t.Errorf("after raising: %d", w.Code)
	}
	// Turning it off cuts the token off.
	_ = setAIApp(Source{App: "agent", Team: "t1"}, false, 0)
	if w := callGateway(a.token, "/openai/v1/chat/completions", `{}`); w.Code != http.StatusUnauthorized {
		t.Errorf("after turning off: %d", w.Code)
	}
}

func TestAIEnvReachesTheAppAndItsPreviews(t *testing.T) {
	openTestDB(t)
	aiGatewayHost.Store("172.17.0.1:7079")
	t.Cleanup(func() { aiGatewayHost = atomic.Value{} })
	if got := aiEnv(Source{App: "agent", Team: "t1"}); got != nil {
		t.Errorf("before it's on: %v", got)
	}
	_ = setAIApp(Source{App: "agent", Team: "t1"}, true, 0)
	a, _ := aiAppFor("agent")
	for _, src := range []Source{{App: "agent"}, {App: "agent-pr-3", Parent: "agent"}} {
		env := map[string]string{}
		for _, e := range aiEnv(src) {
			env[e.Key] = e.Value
		}
		if env["SKIFF_AI_URL"] != "http://172.17.0.1:7079" || env["SKIFF_AI_TOKEN"] != a.token {
			t.Errorf("%s: %v", src.App, env)
		}
	}
}

func TestAIProviderProblem(t *testing.T) {
	for _, c := range []struct {
		name, kind, base, key string
		ok                    bool
	}{
		{"openai", "openai", "", "sk-1", true},
		{"openrouter", "custom", "https://openrouter.ai/api", "k", true},
		{"local", "custom", "http://10.0.0.5:11434", "k", true},
		{"OpenAI", "openai", "", "sk-1", false},
		{"x", "gemini", "", "k", false},
		{"x", "openai", "", "", false},
		{"x", "custom", "", "k", false},
		{"x", "custom", "ftp://host", "k", false},
	} {
		if got := aiProviderProblem(c.name, c.kind, c.base, c.key) == ""; got != c.ok {
			t.Errorf("%+v: ok=%v", c, got)
		}
	}
}
