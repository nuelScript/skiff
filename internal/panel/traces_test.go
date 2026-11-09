package panel

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/otlp"
)

const traceID = "5b8efff798038103d269b633813fc60c"

func TestTracesJoinTheAgentsSpansAndTheGatewaysCalls(t *testing.T) {
	openTestDB(t)
	a := newTestAgent(t, "bot")
	f := &fakeProvider{}
	addProvider(t, "t1", "openai", "openai", f.serve(t).URL, "sk-real-key")
	now := time.Now().UnixNano()
	export := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"bot"}}]},"scopeSpans":[{"spans":[
	{"traceId":"` + traceID + `","spanId":"aaaaaaaaaaaaaaaa","name":"handle issue","startTimeUnixNano":"` + itoa64(now) + `","endTimeUnixNano":"` + itoa64(now+3e9) + `"},
	{"traceId":"` + traceID + `","spanId":"bbbbbbbbbbbbbbbb","parentSpanId":"aaaaaaaaaaaaaaaa","name":"tool search","startTimeUnixNano":"` + itoa64(now+1e8) + `","endTimeUnixNano":"` + itoa64(now+5e8) + `",
	 "attributes":[{"key":"gen_ai.tool.name","value":{"stringValue":"search"}}],"status":{"code":2,"message":"timed out"}}]}]}]}`
	r := httptest.NewRequest(http.MethodPost, "/_skiff/otlp/v1/traces", strings.NewReader(export))
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	(&Panel{}).aiGateway(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "{}" {
		t.Fatalf("export: %d %s", w.Code, w.Body)
	}

	// A model call carrying the trace context lands in the same trace, under the span that made it.
	r = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini"}`))
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("traceparent", "00-"+traceID+"-aaaaaaaaaaaaaaaa-01")
	(&Panel{}).aiGateway(httptest.NewRecorder(), r)
	// One without lands in a trace of its own.
	callGateway(a.token, "/openai/v1/chat/completions", `{"model":"gpt-4o"}`)

	traces := listTraces("bot", 10)
	if len(traces) != 2 {
		t.Fatalf("traces: %+v", traces)
	}
	var run traceSummary
	for _, tr := range traces {
		if tr.ID == traceID {
			run = tr
		}
	}
	if run.Name != "handle issue" || run.Spans != 3 || run.Errors != 1 || run.Input != 100 || run.Output != 20 || run.Models != 1 || run.Tools != 1 || run.Duration != 3000 {
		t.Errorf("run: %+v", run)
	}
	spans := traceSpans("bot", traceID)
	if len(spans) != 3 || spans[0].Name != "handle issue" || spans[0].Parent != "" {
		t.Fatalf("spans: %+v", spans)
	}
	var gw spanView
	for _, s := range spans {
		if s.Source == "gateway" {
			gw = s
		}
	}
	if gw.Parent != "aaaaaaaaaaaaaaaa" || gw.Name != "chat gpt-4o-mini" || gw.Model != "gpt-4o-mini" || gw.Input != 100 {
		t.Errorf("gateway span: %+v", gw)
	}

	// A plain app with AI access keeps no traces.
	_ = setAIApp(Source{App: "web", Team: "t1"}, true, 0)
	web, _ := aiAppFor("web")
	callGateway(web.token, "/openai/v1/chat/completions", `{"model":"gpt-4o"}`)
	if got := listTraces("web", 10); len(got) != 0 {
		t.Errorf("a plain app's traces: %+v", got)
	}
}

func TestOrderSpansReadsAsATree(t *testing.T) {
	got := orderSpans([]spanView{
		{ID: "c", Parent: "a", Start: 3}, {ID: "b", Parent: "a", Start: 2}, {ID: "a", Start: 1},
		{ID: "d", Parent: "b", Start: 4}, {ID: "orphan", Parent: "missing", Start: 0},
	})
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "orphan,a,b,d,c" {
		t.Errorf("order: %v", ids)
	}
}

func TestSpansArePruned(t *testing.T) {
	openTestDB(t)
	now := time.Now()
	old := otlp.Span{TraceID: traceID, SpanID: "0000000000000001", Name: "old", Start: now.Add(-8 * 24 * time.Hour).UnixNano()}
	recent := otlp.Span{TraceID: traceID, SpanID: "0000000000000002", Name: "new", Start: now.UnixNano()}
	bad := otlp.Span{TraceID: "short", SpanID: "x", Name: "bad"}
	if err := storeSpans("bot", []otlp.Span{old, recent, bad}, now); err != nil {
		t.Fatal(err)
	}
	if spans := traceSpans("bot", traceID); len(spans) != 1 || spans[0].Name != "new" {
		t.Errorf("after pruning: %+v", spans)
	}
}

func TestOtelEnv(t *testing.T) {
	env := map[string]string{}
	for _, e := range otelEnv("bot", "172.17.0.1:7079", "skai_x") {
		env[e.Key] = e.Value
	}
	if env["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"] != "http://172.17.0.1:7079/_skiff/otlp/v1/traces" ||
		env["OTEL_EXPORTER_OTLP_TRACES_HEADERS"] != "Authorization=Bearer%20skai_x" || env["OTEL_SERVICE_NAME"] != "bot" {
		t.Errorf("env: %v", env)
	}
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
