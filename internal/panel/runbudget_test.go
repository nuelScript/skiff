package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunBudgetStopsOneRunOnly(t *testing.T) {
	openTestDB(t)
	f := &fakeProvider{}
	addProvider(t, "t1", "openai", "openai", f.serve(t).URL, "sk-real-key")
	if err := setAIApp(Source{App: "bot", Team: "t1"}, true, 0); err != nil {
		t.Fatal(err)
	}
	// Each fake call costs 120 tokens, so a 200-token run gets two calls.
	if err := setRunBudget("bot", 200); err != nil {
		t.Fatal(err)
	}
	a, _ := aiAppFor("bot")
	if a.RunBudget != 200 {
		t.Fatalf("run budget %d", a.RunBudget)
	}
	call := func(header, value string) int {
		r := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(`{"model":"m"}`))
		r.Header.Set("Authorization", "Bearer "+a.token)
		r.Header.Set("Content-Type", "application/json")
		if header != "" {
			r.Header.Set(header, value)
		}
		w := httptest.NewRecorder()
		(&Panel{}).aiGateway(w, r)
		return w.Code
	}
	trace := "00-" + traceID + "-aaaaaaaaaaaaaaaa-01"
	for i, want := range []int{200, 200, 429} {
		if got := call("traceparent", trace); got != want {
			t.Errorf("traced call %d: %d, want %d", i+1, got, want)
		}
	}
	if got := call("X-Skiff-Run", "task-7"); got != 200 {
		t.Errorf("another run: %d", got)
	}
	if got := call("", ""); got != 200 {
		t.Errorf("a call outside any run: %d", got)
	}
	var alerted int
	_ = sqlDB.QueryRow(`SELECT alerted FROM ai_run_usage WHERE app='bot' AND run=?`, traceID).Scan(&alerted)
	if alerted != 1 || runUsed("bot", traceID) != 240 {
		t.Errorf("alerted %d, used %d", alerted, runUsed("bot", traceID))
	}
	// Old runs are forgotten.
	recordRunUse(a, "later", 1, time.Now().Add(runUsageKeep+time.Hour))
	if runUsed("bot", traceID) != 0 {
		t.Error("a week-old run was kept")
	}
}

func TestRunOf(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if runOf(r) != "" {
		t.Error("no run named")
	}
	r.Header.Set("traceparent", "00-"+traceID+"-aaaaaaaaaaaaaaaa-01")
	if runOf(r) != traceID {
		t.Error("trace id")
	}
	r.Header.Set("X-Skiff-Run", "job 1; drop")
	if runOf(r) != traceID {
		t.Error("an unsafe run name should be ignored")
	}
	r.Header.Set("X-Skiff-Run", "job-1")
	if runOf(r) != "job-1" {
		t.Error("the header wins")
	}
}
