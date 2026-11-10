package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nuelScript/skiff/internal/seal"
)

// The AI gateway holds a team's model provider keys, so an app (an agent, say) can call a model without ever holding the key, and stops it at a monthly token budget.
const (
	aiGatewayPort = 7079
	// aiAlertShare is how far into its budget an app is when the team hears it's getting close.
	aiAlertShare = 0.8
	aiMaxBody    = 8 << 20
)

// aiKinds are the providers the gateway knows: where their API lives, and whether it takes the key as x-api-key rather than a bearer token.
var aiKinds = map[string]struct {
	base   string
	apiKey bool
}{
	"openai":    {base: "https://api.openai.com"},
	"anthropic": {base: "https://api.anthropic.com", apiKey: true},
	"custom":    {}, // anything that speaks OpenAI's API, like OpenRouter, Groq or a model on this server
}

var aiNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// aiGatewayHost is where containers reach the gateway, once it's known.
var aiGatewayHost atomic.Value

type aiProvider struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"baseUrl"`
	KeyHint string `json:"keyHint"`
	Created int64  `json:"created"`
	key     string
}

func teamProviders(team string) []aiProvider {
	rows, err := sqlDB.Query(`SELECT name, kind, base_url, api_key, created FROM ai_providers WHERE team=? ORDER BY name`, team)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []aiProvider{}
	for rows.Next() {
		var p aiProvider
		if rows.Scan(&p.Name, &p.Kind, &p.BaseURL, &p.key, &p.Created) == nil && seal.OpenAll(&p.key) == nil {
			if len(p.key) > 8 {
				p.KeyHint = "…" + p.key[len(p.key)-4:]
			}
			out = append(out, p)
		}
	}
	return out
}

func teamProvider(team, name string) (aiProvider, bool) {
	for _, p := range teamProviders(team) {
		if p.Name == name {
			return p, true
		}
	}
	return aiProvider{}, false
}

// aiApp is an app allowed through the gateway, with its token and monthly budget.
type aiApp struct {
	App, Team string
	token     string
	Budget    int64 // tokens a month; 0 means no limit
	RunBudget int64 // tokens one run may use; 0 means no limit
}

func aiAppFor(app string) (aiApp, bool) {
	var a aiApp
	err := sqlDB.QueryRow(`SELECT app, team, token, budget, run_budget FROM ai_apps WHERE app=?`, app).Scan(&a.App, &a.Team, &a.token, &a.Budget, &a.RunBudget)
	if err != nil || seal.OpenAll(&a.token) != nil {
		return aiApp{}, false
	}
	return a, true
}

func aiAppByToken(token string) (aiApp, bool) {
	var app string
	if token == "" || sqlDB.QueryRow(`SELECT app FROM ai_apps WHERE token_hash=?`, hashToken(token)).Scan(&app) != nil {
		return aiApp{}, false
	}
	return aiAppFor(app)
}

// aiEnv gives an app the gateway's address and its token. A preview uses its project's, and its project's budget.
func aiEnv(src Source) []EnvVar {
	app := src.App
	if src.Parent != "" {
		app = src.Parent
	}
	host, _ := aiGatewayHost.Load().(string)
	a, ok := aiAppFor(app)
	if !ok || host == "" {
		return nil
	}
	env := []EnvVar{{Key: "SKIFF_AI_URL", Value: "http://" + host}, {Key: "SKIFF_AI_TOKEN", Value: a.token}}
	if _, agent := getAgent(app); agent {
		env = append(env, otelEnv(app, host, a.token)...)
	}
	return env
}

func aiMonth(t time.Time) string { return t.UTC().Format("2006-01") }

type aiTotals struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Requests int64 `json:"requests"`
}

func aiUsed(app string, now time.Time) aiTotals {
	var t aiTotals
	_ = sqlDB.QueryRow(`SELECT COALESCE(SUM(input),0), COALESCE(SUM(output),0), COALESCE(SUM(requests),0) FROM ai_usage WHERE app=? AND month=?`,
		app, aiMonth(now)).Scan(&t.Input, &t.Output, &t.Requests)
	return t
}

func recordAIUse(a aiApp, provider, run string, u aiUsage, now time.Time) {
	if run != "" {
		recordRunUse(a, run, u.in+u.out, now)
	}
	_, _ = sqlDB.Exec(`INSERT INTO ai_usage(app, month, provider, requests, input, output) VALUES(?,?,?,1,?,?)
		ON CONFLICT(app, month, provider) DO UPDATE SET requests=requests+1, input=input+excluded.input, output=output+excluded.output`,
		a.App, aiMonth(now), provider, u.in, u.out)
	if a.Budget <= 0 {
		return
	}
	t := aiUsed(a.App, now)
	used := t.Input + t.Output
	before := used - u.in - u.out
	for _, level := range []float64{aiAlertShare, 1} {
		mark := int64(float64(a.Budget) * level)
		if before >= mark || used < mark {
			continue
		}
		title := fmt.Sprintf("%s has used %d%% of its AI budget", a.App, int(level*100))
		detail := fmt.Sprintf("%d of %d tokens this month.", used, a.Budget)
		if level == 1 {
			title = a.App + " reached its AI budget"
			detail += " Skiff refuses its model calls until next month, or until you raise the budget."
		}
		go dispatchAlert(alertEvent{Team: a.Team, Kind: "ai.budget", App: a.App, Title: title, Detail: detail})
	}
}

// aiUsage is the token count in a response. OpenAI calls them prompt and completion tokens, Anthropic input and output. Anthropic's stream puts the input count under message, and the Responses API's under response. Anthropic counts cached input apart from the rest, so it's added back.
type aiUsage struct{ in, out int64 }

type aiCounts struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadTokens     int64 `json:"cache_read_input_tokens"`
}

func (u *aiUsage) add(c aiCounts) {
	u.in = max(u.in, c.PromptTokens, c.InputTokens+c.CacheCreationTokens+c.CacheReadTokens)
	u.out = max(u.out, c.CompletionTokens, c.OutputTokens)
}

// take reads the counts in a response or stream event; false when it isn't whole JSON.
func (u *aiUsage) take(raw []byte) bool {
	var v struct {
		Usage    *aiCounts `json:"usage"`
		Message  *aiNested `json:"message"`
		Response *aiNested `json:"response"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	for _, c := range []*aiCounts{v.Usage, v.Message.usage(), v.Response.usage()} {
		if c != nil {
			u.add(*c)
		}
	}
	return true
}

type aiNested struct {
	Usage *aiCounts `json:"usage"`
}

func (n *aiNested) usage() *aiCounts {
	if n == nil {
		return nil
	}
	return n.Usage
}

var usageField = regexp.MustCompile(`"(prompt_tokens|completion_tokens|input_tokens|output_tokens|cache_creation_input_tokens|cache_read_input_tokens)"\s*:\s*(\d+)`)

// scan picks the counts out of a body too big to read whole; the usage is at the end of one.
func (u *aiUsage) scan(raw []byte) {
	var c aiCounts
	for _, m := range usageField.FindAllSubmatch(raw, -1) {
		n, _ := strconv.ParseInt(string(m[2]), 10, 64)
		switch string(m[1]) {
		case "prompt_tokens":
			c.PromptTokens = max(c.PromptTokens, n)
		case "completion_tokens":
			c.CompletionTokens = max(c.CompletionTokens, n)
		case "input_tokens":
			c.InputTokens = max(c.InputTokens, n)
		case "output_tokens":
			c.OutputTokens = max(c.OutputTokens, n)
		case "cache_creation_input_tokens":
			c.CacheCreationTokens = max(c.CacheCreationTokens, n)
		case "cache_read_input_tokens":
			c.CacheReadTokens = max(c.CacheReadTokens, n)
		}
	}
	u.add(c)
}

// streamedText is the text a stream event carries, for estimating a stream cut off before its counts: an OpenAI chat delta, an Anthropic text delta, or a Responses API delta.
func streamedText(raw []byte) int {
	var v struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
		Delta any `json:"delta"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return 0
	}
	n := 0
	for _, c := range v.Choices {
		n += len(c.Delta.Content)
	}
	switch d := v.Delta.(type) {
	case string:
		n += len(d)
	case map[string]any:
		if t, ok := d["text"].(string); ok {
			n += len(t)
		}
	}
	return n
}

// aiMeter passes a response through while reading the usage in it: a stream event by event, a JSON body whole. A body past aiMaxBody keeps its end, where the usage is. A stream cut off before its counts is estimated at four characters a token, from the request's size and the text it streamed, since the provider charges for it all the same.
type aiMeter struct {
	body     io.ReadCloser
	stream   bool
	buf      bytes.Buffer
	tail     []byte
	u        aiUsage
	sent     int // bytes in the request
	streamed int // characters of text streamed
	done     func(aiUsage)
	once     sync.Once
}

const aiTailKeep = 64 << 10

func (m *aiMeter) Read(p []byte) (int, error) {
	n, err := m.body.Read(p)
	if n > 0 {
		m.note(p[:n])
	}
	if err == io.EOF {
		m.finish()
	}
	return n, err
}

func (m *aiMeter) note(b []byte) {
	if !m.stream {
		if m.buf.Len() < aiMaxBody {
			m.buf.Write(b)
		} else {
			m.tail = append(m.tail, b...)
			if len(m.tail) > 2*aiTailKeep {
				m.tail = append(m.tail[:0], m.tail[len(m.tail)-aiTailKeep:]...)
			}
		}
		return
	}
	m.buf.Write(b)
	for {
		line, err := m.buf.ReadBytes('\n')
		if err != nil {
			// An unfinished line waits for the rest, unless it's far longer than any usage event.
			if len(line) < 1<<20 {
				m.buf.Write(line)
			}
			return
		}
		m.event(line)
	}
}

func (m *aiMeter) event(line []byte) {
	if data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:")); ok {
		data = bytes.TrimSpace(data)
		m.u.take(data)
		m.streamed += streamedText(data)
	}
}

func (m *aiMeter) Close() error {
	m.finish()
	return m.body.Close()
}

func (m *aiMeter) finish() {
	m.once.Do(func() {
		if m.stream {
			m.event(m.buf.Bytes())
			if m.u.in == 0 && m.sent > 0 {
				m.u.in = int64(m.sent/4 + 1)
			}
			if m.u.out == 0 && m.streamed > 0 {
				m.u.out = int64(m.streamed/4 + 1)
			}
		} else if !m.u.take(m.buf.Bytes()) {
			m.u.scan(append(m.buf.Bytes(), m.tail...))
		}
		m.done(m.u)
	})
}

// gatewayToken finds the app's token wherever its SDK put it.
func gatewayToken(r *http.Request) string {
	if t := r.Header.Get("x-api-key"); t != "" {
		return t
	}
	t, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return strings.TrimSpace(t)
}

func gatewayError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "skiff_gateway", "message": msg}})
}

// peekRequest reads the model a JSON request names, and its size. For OpenAI it also makes a stream end with its token count, which OpenAI leaves out unless asked.
func peekRequest(r *http.Request, kind string) (model string, size int) {
	if r.Body == nil || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return "", int(max(r.ContentLength, 0))
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, aiMaxBody+1))
	_ = r.Body.Close()
	body := raw
	var v map[string]any
	if err == nil && len(raw) <= aiMaxBody && json.Unmarshal(raw, &v) == nil {
		model, _ = v["model"].(string)
		if kind == "openai" && v["stream"] == true {
			opts, _ := v["stream_options"].(map[string]any)
			if opts == nil {
				opts = map[string]any{}
			}
			opts["include_usage"] = true
			v["stream_options"] = opts
			if b, err := json.Marshal(v); err == nil {
				body = b
			}
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	return model, len(raw)
}

// aiGateway forwards /<provider>/<path> to that provider with the team's key, after checking the app's token and budget.
func (p *Panel) aiGateway(w http.ResponseWriter, r *http.Request) {
	a, ok := aiAppByToken(gatewayToken(r))
	if !ok {
		gatewayError(w, http.StatusUnauthorized, "unknown SKIFF_AI_TOKEN; redeploy the app if its AI access was just turned on")
		return
	}
	if agentStopped(a.App) {
		gatewayError(w, http.StatusForbidden, a.App+" was stopped in Skiff; start it again to continue")
		return
	}
	name, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if name == "_skiff" {
		p.agentAPI(w, r, a, rest)
		return
	}
	pr, ok := teamProvider(a.Team, name)
	if !ok {
		gatewayError(w, http.StatusNotFound, fmt.Sprintf("your team has no AI provider named %q; add one in Skiff under Settings → Integrations", name))
		return
	}
	if a.Budget > 0 {
		if t := aiUsed(a.App, time.Now()); t.Input+t.Output >= a.Budget {
			gatewayError(w, http.StatusTooManyRequests, "this app has used its monthly AI budget; raise it in Skiff to continue")
			return
		}
	}
	run := runOf(r)
	if a.RunBudget > 0 && run != "" && runUsed(a.App, run) >= a.RunBudget {
		gatewayError(w, http.StatusTooManyRequests, fmt.Sprintf("this run has used its budget of %d tokens; start a new run, or raise the per-run budget in Skiff", a.RunBudget))
		return
	}
	base := pr.BaseURL
	if base == "" {
		base = aiKinds[pr.Kind].base
	}
	target, err := url.Parse(base)
	if err != nil || target.Host == "" {
		gatewayError(w, http.StatusBadGateway, "the provider's address in Skiff is invalid")
		return
	}
	started := time.Now()
	model, sent := peekRequest(r, pr.Kind)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.Out.URL.Scheme, req.Out.URL.Host, req.Out.Host = target.Scheme, target.Host, target.Host
			req.Out.URL.Path = strings.TrimSuffix(target.Path, "/") + "/" + rest
			req.Out.URL.RawPath = ""
			req.Out.Header.Del("Authorization")
			req.Out.Header.Del("x-api-key")
			// Go's transport then asks for gzip itself and unpacks the reply, so the usage in it can be counted.
			req.Out.Header.Del("Accept-Encoding")
			if aiKinds[pr.Kind].apiKey {
				req.Out.Header.Set("x-api-key", pr.key)
			} else {
				req.Out.Header.Set("Authorization", "Bearer "+pr.key)
			}
		},
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode >= 300 {
				gatewaySpan(a, r, pr.Name, pr.Kind, model, started, resp.StatusCode, aiUsage{}, "")
				return nil
			}
			status := resp.StatusCode
			resp.Body = &aiMeter{
				body:   resp.Body,
				sent:   sent,
				stream: strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"),
				done: func(u aiUsage) {
					recordAIUse(a, pr.Name, run, u, time.Now())
					gatewaySpan(a, r, pr.Name, pr.Kind, model, started, status, u, "")
				},
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			gatewaySpan(a, r, pr.Name, pr.Kind, model, started, http.StatusBadGateway, aiUsage{}, err.Error())
			gatewayError(w, http.StatusBadGateway, "couldn't reach "+target.Host+": "+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}

// aiGatewayAddr is the default Docker bridge's address on this server, which every container can reach.
func (p *Panel) aiGatewayAddr() string {
	ip := p.eng.BridgeGateway()
	if ip == "" {
		return ""
	}
	return net.JoinHostPort(ip, fmt.Sprint(aiGatewayPort))
}

// aiGatewayLoop serves the gateway. While a new panel takes over from the old one the port is still held, so it keeps trying.
func (p *Panel) aiGatewayLoop() {
	addr := p.aiGatewayAddr()
	if addr == "" {
		return
	}
	aiGatewayHost.Store(addr)
	allowGatewayIn(addr)
	srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(p.aiGateway), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	logged := ""
	for {
		err := srv.ListenAndServe()
		if err != nil && err.Error() != logged && !strings.Contains(err.Error(), "address already in use") {
			log.Printf("skiff: AI gateway: %v", err)
			logged = err.Error()
		}
		time.Sleep(5 * time.Second)
	}
}

// allowGatewayIn lets containers reach the gateway past a host firewall like UFW. The address is only reachable from this server's containers, and every request needs an app's token.
func allowGatewayIn(addr string) {
	if runtime.GOOS != "linux" {
		return
	}
	host, port, _ := net.SplitHostPort(addr)
	rule := []string{"INPUT", "-d", host, "-p", "tcp", "--dport", port, "-j", "ACCEPT"}
	if exec.Command("iptables", append([]string{"-w", "-C"}, rule...)...).Run() != nil {
		_ = exec.Command("iptables", append([]string{"-w", "-I"}, rule...)...).Run()
	}
}

func (p *Panel) handleAIProviders(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	if r.Method != http.MethodGet && !p.isOwner(r) {
		http.Error(w, "only a team owner can change AI providers", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"providers": teamProviders(team), "gateway": aiGatewayHost.Load() != nil})
	case http.MethodPost:
		var body struct {
			Name    string `json:"name"`
			Kind    string `json:"kind"`
			BaseURL string `json:"baseUrl"`
			Key     string `json:"key"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body.Name, body.Key, body.BaseURL = strings.TrimSpace(body.Name), strings.TrimSpace(body.Key), strings.TrimSpace(body.BaseURL)
		if problem := aiProviderProblem(body.Name, body.Kind, body.BaseURL, body.Key); problem != "" {
			http.Error(w, problem, http.StatusBadRequest)
			return
		}
		sealed, err := seal.Seal(body.Key)
		if err != nil {
			http.Error(w, "couldn't save the key", http.StatusInternalServerError)
			return
		}
		if _, err := sqlDB.Exec(`INSERT INTO ai_providers(team, name, kind, base_url, api_key, created) VALUES(?,?,?,?,?,?)
			ON CONFLICT(team, name) DO UPDATE SET kind=excluded.kind, base_url=excluded.base_url, api_key=excluded.api_key`,
			team, body.Name, body.Kind, strings.TrimSuffix(body.BaseURL, "/"), sealed, time.Now().Unix()); err != nil {
			http.Error(w, "couldn't save", http.StatusInternalServerError)
			return
		}
		p.audit(r, "ai.provider.save", body.Name, body.Kind)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if _, err := sqlDB.Exec(`DELETE FROM ai_providers WHERE team=? AND name=?`, team, name); err != nil {
			http.Error(w, "couldn't remove", http.StatusInternalServerError)
			return
		}
		p.audit(r, "ai.provider.remove", name, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func aiProviderProblem(name, kind, baseURL, key string) string {
	if !aiNameRe.MatchString(name) {
		return "name the provider with lowercase letters, numbers and dashes, like openai"
	}
	if _, ok := aiKinds[kind]; !ok {
		return "choose OpenAI, Anthropic, or another OpenAI-compatible provider"
	}
	if key == "" {
		return "paste the provider's API key"
	}
	if kind == "custom" && baseURL == "" {
		return "give the provider's API address, like https://openrouter.ai/api"
	}
	if baseURL != "" {
		u, err := url.Parse(baseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return "the API address must start with https://"
		}
	}
	return ""
}

func (p *Panel) handleProjectAI(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		a, on := aiAppFor(app)
		providers := []string{}
		for _, pr := range teamProviders(src.Team) {
			providers = append(providers, pr.Name)
		}
		writeJSON(w, map[string]any{
			"enabled": on, "budget": a.Budget, "runBudget": a.RunBudget, "used": aiUsed(app, time.Now()),
			"providers": providers, "gateway": aiGatewayHost.Load() != nil,
		})
	case http.MethodPut:
		if !p.isOwner(r) {
			http.Error(w, "only a team owner can give an app AI access", http.StatusForbidden)
			return
		}
		if src.Parent != "" {
			http.Error(w, "previews use their project's AI access", http.StatusBadRequest)
			return
		}
		var body struct {
			Enabled   bool   `json:"enabled"`
			Budget    int64  `json:"budget"`
			RunBudget *int64 `json:"runBudget"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || body.Budget < 0 || body.RunBudget != nil && *body.RunBudget < 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := setAIApp(src, body.Enabled, body.Budget); err != nil {
			http.Error(w, "couldn't save", http.StatusInternalServerError)
			return
		}
		if body.Enabled && body.RunBudget != nil {
			if err := setRunBudget(app, *body.RunBudget); err != nil {
				http.Error(w, "couldn't save", http.StatusInternalServerError)
				return
			}
		}
		p.audit(r, "project.ai", app, fmt.Sprintf("enabled=%v budget=%d", body.Enabled, body.Budget))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// setAIApp turns an app's gateway access on or off. Its token stays the same while it's on, so a budget change needs no redeploy.
func setAIApp(src Source, enabled bool, budget int64) error {
	if !enabled {
		_, err := sqlDB.Exec(`DELETE FROM ai_apps WHERE app=?`, src.App)
		return err
	}
	if _, on := aiAppFor(src.App); on {
		_, err := sqlDB.Exec(`UPDATE ai_apps SET budget=? WHERE app=?`, budget, src.App)
		return err
	}
	token := "skai_" + randHex(24)
	sealed, err := seal.Seal(token)
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(`INSERT INTO ai_apps(app, team, token, token_hash, budget) VALUES(?,?,?,?,?)`, src.App, src.Team, sealed, hashToken(token), budget)
	return err
}
