package panel

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nuelScript/skiff/internal/otlp"
)

// Traces are an agent's runs, step by step: the spans its own OpenTelemetry instrumentation sends, and one for every model call through the gateway, joined into the same trace when the call carries its trace context.
const (
	traceKeep      = 7 * 24 * time.Hour
	traceMaxSpans  = 20000 // per app
	traceMaxAttrs  = 8 << 10
	traceMaxName   = 256
	traceMaxText   = 2 << 10
	traceListLimit = 30
	otlpPath       = "otlp/v1/traces"
)

var traceparentRe = regexp.MustCompile(`^[0-9a-f]{2}-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$`)

// otelEnv points an agent's OpenTelemetry SDK at Skiff, so an instrumented agent sends its traces with no setup. The app's own variables still win.
func otelEnv(app, host, token string) []EnvVar {
	base := "http://" + host + "/_skiff/" + otlpPath
	return []EnvVar{
		{Key: "OTEL_SERVICE_NAME", Value: app},
		{Key: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", Value: base},
		{Key: "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", Value: "http/protobuf"},
		{Key: "OTEL_EXPORTER_OTLP_TRACES_HEADERS", Value: "Authorization=" + url.PathEscape("Bearer "+token)},
	}
}

// spanFacts pulls out what the run view shows, from the GenAI conventions and the older names some libraries still send.
func spanFacts(attrs map[string]any) (model, tool string, in, out int64) {
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := attrs[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	num := func(keys ...string) int64 {
		for _, k := range keys {
			switch v := attrs[k].(type) {
			case int64:
				return v
			case float64:
				return int64(v)
			}
		}
		return 0
	}
	model = str("gen_ai.response.model", "gen_ai.request.model", "llm.response.model", "llm.request.model")
	tool = str("gen_ai.tool.name", "tool.name")
	in = num("gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens", "llm.usage.prompt_tokens")
	out = num("gen_ai.usage.output_tokens", "gen_ai.usage.completion_tokens", "llm.usage.completion_tokens")
	return
}

func storeSpans(app string, spans []otlp.Span, now time.Time) error {
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO spans(app, trace_id, span_id, parent_id, name, start_ns, end_ns, status, status_msg, model, tool, input_tokens, output_tokens, attrs, source)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range spans {
		if len(s.TraceID) != 32 || len(s.SpanID) != 16 {
			continue
		}
		// A clock running ahead, or a made-up date, would keep a span past every cut-off.
		if limit := now.UnixNano(); s.Start > limit {
			s.End, s.Start = limit+max(s.End-s.Start, 0), limit
		}
		s.Name, s.StatusMessage = clip(s.Name, traceMaxName), clip(s.StatusMessage, traceMaxText)
		model, tool, in, out := spanFacts(s.Attrs)
		model, tool = clip(model, traceMaxName), clip(tool, traceMaxName)
		attrs, _ := json.Marshal(s.Attrs)
		if len(attrs) > traceMaxAttrs {
			attrs = []byte(`{"skiff.note":"attributes over 8 KB were left out"}`)
		}
		source := "app"
		if s.Attrs["skiff.gateway"] == true {
			source = "gateway"
		}
		if _, err := stmt.Exec(app, s.TraceID, s.SpanID, s.ParentID, s.Name, s.Start, max(s.End, s.Start), s.StatusCode, s.StatusMessage,
			model, tool, in, out, string(attrs), source); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	pruneSpans(app, now)
	return nil
}

// clip cuts s to at most n bytes without splitting a character.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// The gateway records every model call made through it. When an app's own span for that call arrived too, the gateway's span sits beneath it, and only the gateway's tokens count. shadowed lists those app spans, by trace.
const shadowed = `WITH RECURSIVE shadowed(trace_id, span_id) AS (
	SELECT trace_id, parent_id FROM spans WHERE app=? AND source='gateway' AND parent_id<>''
	UNION SELECT s.trace_id, s.parent_id FROM shadowed JOIN spans s ON s.app=? AND s.trace_id=shadowed.trace_id AND s.span_id=shadowed.span_id AND s.parent_id<>'')
`

const countedSpan = `NOT EXISTS (SELECT 1 FROM shadowed WHERE shadowed.trace_id=spans.trace_id AND shadowed.span_id=spans.span_id)`

func pruneSpans(app string, now time.Time) {
	_, _ = sqlDB.Exec(`DELETE FROM spans WHERE app=? AND start_ns<?`, app, now.Add(-traceKeep).UnixNano())
	_, _ = sqlDB.Exec(`DELETE FROM spans WHERE app=? AND rowid IN (SELECT rowid FROM spans WHERE app=? ORDER BY start_ns DESC LIMIT -1 OFFSET ?)`, app, app, traceMaxSpans)
}

// handleOTLP takes an OpenTelemetry trace export from an app on the gateway.
func handleOTLP(w http.ResponseWriter, r *http.Request, a aiApp) {
	if r.Method != http.MethodPost {
		gatewayError(w, http.StatusMethodNotAllowed, "send traces with POST")
		return
	}
	spans, err := otlp.Read(r.Body, r.Header.Get("Content-Type"), r.Header.Get("Content-Encoding"))
	if err != nil {
		gatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := storeSpans(a.App, spans, time.Now()); err != nil {
		gatewayError(w, http.StatusInternalServerError, "couldn't keep the traces")
		return
	}
	// An empty ExportTraceServiceResponse, in the encoding that was sent.
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// gatewaySpan records a model call as a span, inside the caller's trace when it sent a traceparent header.
func gatewaySpan(a aiApp, r *http.Request, provider, kind, model string, start time.Time, status int, u aiUsage, failure string) {
	if _, ok := getAgent(a.App); !ok {
		return
	}
	trace, parent := randomHex(16), ""
	if m := traceparentRe.FindStringSubmatch(strings.ToLower(r.Header.Get("traceparent"))); m != nil {
		trace, parent = m[1], m[2]
	}
	name := provider + " " + strings.TrimPrefix(r.URL.Path, "/"+provider)
	if model != "" {
		name = "chat " + model
	}
	attrs := map[string]any{
		"skiff.gateway": true, "skiff.provider": provider, "gen_ai.system": kind,
		"http.response.status_code": int64(status), "url.path": r.URL.Path,
	}
	if model != "" {
		attrs["gen_ai.request.model"] = model
	}
	if u.in > 0 || u.out > 0 {
		attrs["gen_ai.usage.input_tokens"], attrs["gen_ai.usage.output_tokens"] = u.in, u.out
	}
	code := 1
	if status >= 400 || failure != "" {
		code = 2
		if failure == "" {
			failure = fmt.Sprintf("the provider answered %d", status)
		}
	}
	_ = storeSpans(a.App, []otlp.Span{{TraceID: trace, SpanID: randomHex(8), ParentID: parent, Name: name, Kind: 3,
		Start: start.UnixNano(), End: time.Now().UnixNano(), Attrs: attrs, StatusCode: code, StatusMessage: failure}}, time.Now())
}

type traceSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Start    int64  `json:"start"` // unix ms
	Duration int64  `json:"durationMs"`
	Spans    int    `json:"spans"`
	Errors   int    `json:"errors"`
	Input    int64  `json:"inputTokens"`
	Output   int64  `json:"outputTokens"`
	Models   int    `json:"modelCalls"`
	Tools    int    `json:"toolCalls"`
	// Cost is in dollars, from the team's model prices; nil when no model in the run has one, and Partial when only some do.
	Cost    *float64 `json:"cost,omitempty"`
	Partial bool     `json:"costPartial,omitempty"`
}

type spanView struct {
	ID      string         `json:"id"`
	Parent  string         `json:"parent"`
	Name    string         `json:"name"`
	Start   int64          `json:"startNs"`
	End     int64          `json:"endNs"`
	Status  int            `json:"status"`
	Message string         `json:"message,omitempty"`
	Model   string         `json:"model,omitempty"`
	Tool    string         `json:"tool,omitempty"`
	Input   int64          `json:"inputTokens,omitempty"`
	Output  int64          `json:"outputTokens,omitempty"`
	Source  string         `json:"source"`
	Attrs   map[string]any `json:"attrs"`
	Cost    *float64       `json:"cost,omitempty"`
}

func listTraces(app string, limit int, prices []aiPrice) []traceSummary {
	rows, err := sqlDB.Query(shadowed+`SELECT trace_id, MIN(start_ns), MAX(end_ns), COUNT(*), SUM(status=2),
		SUM(CASE WHEN `+countedSpan+` THEN input_tokens ELSE 0 END), SUM(CASE WHEN `+countedSpan+` THEN output_tokens ELSE 0 END),
		SUM(model<>'' OR source='gateway'), SUM(tool<>'') FROM spans WHERE app=? GROUP BY trace_id ORDER BY MIN(start_ns) DESC LIMIT ?`, app, app, app, limit)
	if err != nil {
		return nil
	}
	out := []traceSummary{}
	for rows.Next() {
		var t traceSummary
		var start, end int64
		if rows.Scan(&t.ID, &start, &end, &t.Spans, &t.Errors, &t.Input, &t.Output, &t.Models, &t.Tools) == nil {
			t.Start, t.Duration = start/1e6, (end-start)/1e6
			out = append(out, t)
		}
	}
	rows.Close()
	// A trace's name is its root's: the span with no parent, or whose parent never arrived.
	for i := range out {
		_ = sqlDB.QueryRow(`SELECT name FROM spans s WHERE app=? AND trace_id=? AND (parent_id='' OR NOT EXISTS
			(SELECT 1 FROM spans p WHERE p.app=s.app AND p.trace_id=s.trace_id AND p.span_id=s.parent_id)) ORDER BY start_ns LIMIT 1`,
			app, out[i].ID).Scan(&out[i].Name)
		out[i].Cost, out[i].Partial = traceCost(app, out[i].ID, prices)
	}
	return out
}

// traceCost adds up a run's model calls at the team's prices.
func traceCost(app, trace string, prices []aiPrice) (*float64, bool) {
	if len(prices) == 0 {
		return nil, false
	}
	rows, err := sqlDB.Query(shadowed+`SELECT model, SUM(input_tokens), SUM(output_tokens) FROM spans WHERE app=? AND trace_id=? AND model<>'' AND `+countedSpan+` GROUP BY model`, app, app, app, trace)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	var total float64
	priced, unpriced := 0, 0
	for rows.Next() {
		var model string
		var in, out int64
		if rows.Scan(&model, &in, &out) != nil {
			continue
		}
		if p, ok := priceFor(prices, model); ok {
			total += costOf(p, in, out)
			priced++
		} else if in+out > 0 {
			unpriced++
		}
	}
	if priced == 0 {
		return nil, false
	}
	return &total, unpriced > 0
}

func traceSpans(app, trace string, prices []aiPrice) []spanView {
	rows, err := sqlDB.Query(`SELECT span_id, parent_id, name, start_ns, end_ns, status, status_msg, model, tool, input_tokens, output_tokens, source, attrs
		FROM spans WHERE app=? AND trace_id=? ORDER BY start_ns`, app, trace)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []spanView{}
	for rows.Next() {
		var s spanView
		var attrs string
		if rows.Scan(&s.ID, &s.Parent, &s.Name, &s.Start, &s.End, &s.Status, &s.Message, &s.Model, &s.Tool, &s.Input, &s.Output, &s.Source, &attrs) == nil {
			_ = json.Unmarshal([]byte(attrs), &s.Attrs)
			if p, ok := priceFor(prices, s.Model); ok && s.Model != "" && s.Input+s.Output > 0 {
				c := costOf(p, s.Input, s.Output)
				s.Cost = &c
			}
			out = append(out, s)
		}
	}
	return orderSpans(out)
}

// orderSpans puts each span after its parent, in the order they started, so the list reads as a tree.
func orderSpans(spans []spanView) []spanView {
	byID := map[string]bool{}
	for _, s := range spans {
		byID[s.ID] = true
	}
	kids := map[string][]spanView{}
	var roots []spanView
	for _, s := range spans {
		if s.Parent == "" || !byID[s.Parent] {
			roots = append(roots, s)
		} else {
			kids[s.Parent] = append(kids[s.Parent], s)
		}
	}
	out := make([]spanView, 0, len(spans))
	var walk func(list []spanView)
	walk = func(list []spanView) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Start < list[j].Start })
		for _, s := range list {
			out = append(out, s)
			walk(kids[s.ID])
		}
	}
	walk(roots)
	return out
}

func (p *Panel) handleTraces(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	prices := teamPrices(src.Team)
	if trace := strings.ToLower(r.URL.Query().Get("trace")); trace != "" {
		writeJSON(w, map[string]any{"spans": traceSpans(app, trace, prices)})
		return
	}
	writeJSON(w, map[string]any{"traces": listTraces(app, traceListLimit, prices)})
}
