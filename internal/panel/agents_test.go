package panel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/hooksig"
)

func newTestAgent(t *testing.T, app string) aiApp {
	t.Helper()
	src := Source{App: app, Team: "t1", Port: "3000"}
	applyAgentPreset(&src)
	if err := putSource(src); err != nil {
		t.Fatal(err)
	}
	if err := setupAgent(src); err != nil {
		t.Fatal(err)
	}
	a, ok := aiAppFor(app)
	if !ok {
		t.Fatal("an agent should get a gateway token")
	}
	return a
}

func TestAgentPresetSetsUpStateAndBudget(t *testing.T) {
	openTestDB(t)
	a := newTestAgent(t, "bot")
	src, _ := getSource("bot")
	if !src.Private || src.HealthMode != "none" {
		t.Errorf("preset: private=%v health=%q", src.Private, src.HealthMode)
	}
	if vols := listVolumes("bot"); len(vols) != 1 || vols[0].Path != agentStatePath {
		t.Errorf("volumes: %+v", vols)
	}
	if a.Budget != agentBudget {
		t.Errorf("budget %d", a.Budget)
	}
	// Setting it up again changes nothing.
	if err := setupAgent(src); err != nil || len(listVolumes("bot")) != 1 {
		t.Errorf("second setup: %v, %d volumes", err, len(listVolumes("bot")))
	}
}

func TestAgentInboxLeasesAndRedelivers(t *testing.T) {
	openTestDB(t)
	newTestAgent(t, "bot")
	now := time.Now()
	first, _ := addAgentEvent("bot", "webhook", "", `{"n":1}`, now)
	second, _ := addAgentEvent("bot", "webhook", "", `{"n":2}`, now)

	e, ok := takeAgentEvent("bot", now)
	if !ok || e.ID != first || e.Deliveries != 1 {
		t.Fatalf("first take: %+v %v", e, ok)
	}
	// While it's leased, the next one comes out.
	if e, _ := takeAgentEvent("bot", now); e.ID != second {
		t.Errorf("second take: %+v", e)
	}
	if _, ok := takeAgentEvent("bot", now); ok {
		t.Error("nothing should be left while both are leased")
	}
	// The agent finished the second but crashed on the first, which comes back when its lease runs out.
	if !finishAgentEvent("bot", second) {
		t.Fatal("finish")
	}
	if e, ok := takeAgentEvent("bot", now.Add(agentLease+time.Second)); !ok || e.ID != first || e.Deliveries != 2 {
		t.Errorf("after the lease: %+v %v", e, ok)
	}
	if finishAgentEvent("other", first) {
		t.Error("another app finished this agent's event")
	}
}

func TestAgentInboxThroughTheGateway(t *testing.T) {
	openTestDB(t)
	a := newTestAgent(t, "bot")
	call := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Authorization", "Bearer "+a.token)
		w := httptest.NewRecorder()
		(&Panel{}).aiGateway(w, r)
		return w
	}
	if w := call(http.MethodGet, "/_skiff/events"); w.Code != http.StatusNoContent {
		t.Errorf("empty inbox: %d", w.Code)
	}
	id, _ := addAgentEvent("bot", "github", "issues", `{"action":"opened"}`, time.Now())
	w := call(http.MethodGet, "/_skiff/events?wait=5")
	var e agentEvent
	if json.Unmarshal(w.Body.Bytes(), &e) != nil || e.ID != id || e.Kind != "issues" || e.Body != `{"action":"opened"}` {
		t.Fatalf("event: %d %s", w.Code, w.Body)
	}
	if w := call(http.MethodPost, "/_skiff/events/"+strconv.FormatInt(id, 10)+"/done"); w.Code != http.StatusNoContent {
		t.Errorf("done: %d", w.Code)
	}

	// A plain app with a token has no inbox.
	_ = setAIApp(Source{App: "web", Team: "t1"}, true, 0)
	web, _ := aiAppFor("web")
	r := httptest.NewRequest(http.MethodGet, "/_skiff/events", nil)
	r.Header.Set("Authorization", "Bearer "+web.token)
	rec := httptest.NewRecorder()
	(&Panel{}).aiGateway(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a plain app's inbox: %d", rec.Code)
	}

	// Stopped, the agent can't read its inbox or call a model.
	_, _ = sqlDB.Exec(`UPDATE agents SET stopped=1 WHERE app='bot'`)
	if w := call(http.MethodGet, "/_skiff/events"); w.Code != http.StatusForbidden {
		t.Errorf("stopped: %d", w.Code)
	}
}

func TestAgentHookTakesSignedEvents(t *testing.T) {
	openTestDB(t)
	newTestAgent(t, "bot")
	ag, _ := getAgent("bot")
	p := &Panel{}
	send := func(body string, headers map[string]string, secret string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/hooks/agents/bot", strings.NewReader(body))
		r.SetPathValue("app", "bot")
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		r.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		p.handleAgentHook(w, r)
		return w.Code
	}
	if code := send(`{}`, nil, "wrong"); code != http.StatusUnauthorized {
		t.Errorf("bad signature: %d", code)
	}
	if code := send(`{}`, map[string]string{"X-GitHub-Event": "ping"}, ag.hookSecret); code != http.StatusOK {
		t.Errorf("ping: %d", code)
	}
	if code := send(`{"pull_request":{}}`, map[string]string{"X-GitHub-Event": "pull_request"}, ag.hookSecret); code != http.StatusAccepted {
		t.Errorf("github event: %d", code)
	}
	if code := send(`{"text":"hi"}`, nil, ag.hookSecret); code != http.StatusAccepted {
		t.Errorf("plain webhook: %d", code)
	}
	events := recentAgentEvents("bot", 10)
	if len(events) != 2 || events[1].Source != "github" || events[1].Kind != "pull_request" || events[0].Source != "webhook" {
		t.Errorf("events: %+v", events)
	}
}

func TestAgentScheduleTicksOnce(t *testing.T) {
	openTestDB(t)
	newTestAgent(t, "bot")
	start := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	_, _ = sqlDB.Exec(`UPDATE agents SET schedule='*/15 * * * *', last_tick=?, created=? WHERE app='bot'`, start.Unix(), start.Unix())
	tickAgents(start.Add(10 * time.Minute))
	if n := len(recentAgentEvents("bot", 10)); n != 0 {
		t.Errorf("before its time: %d events", n)
	}
	// Two hours down makes one event, not eight.
	tickAgents(start.Add(2 * time.Hour))
	tickAgents(start.Add(2*time.Hour + time.Minute))
	if events := recentAgentEvents("bot", 10); len(events) != 1 || events[0].Source != "schedule" {
		t.Errorf("after a gap: %+v", events)
	}
}

func TestAgentMailDestinations(t *testing.T) {
	openTestDB(t)
	newTestAgent(t, "bot")
	ag, _ := getAgent("bot")
	got := routerDestinations([]string{"me@example.com", "agent:bot", "agent:gone"})
	if len(got) != 2 || got[0] != "me@example.com" || got[1] != "agent:bot:"+ag.hookSecret {
		t.Errorf("router destinations: %v", got)
	}

	// The router's delivery arrives as an email event.
	body := `{"from":"ada@example.com","subject":"hi","text":"hello"}`
	r := httptest.NewRequest(http.MethodPost, "/api/hooks/agents/bot", strings.NewReader(body))
	r.SetPathValue("app", "bot")
	hooksig.Set(r.Header, ag.hookSecret, []byte(body))
	r.Header.Set("X-Skiff-Source", "email")
	r.Header.Set("X-Skiff-Event", "email")
	w := httptest.NewRecorder()
	(&Panel{}).handleAgentHook(w, r)
	if events := recentAgentEvents("bot", 5); w.Code != http.StatusAccepted || len(events) != 1 || events[0].Source != "email" {
		t.Errorf("%d %+v", w.Code, events)
	}
}
