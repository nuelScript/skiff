package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/nuelScript/skiff/internal/registry"
)

// Alerting closes the loop on everything we watch: a control loop plus deploy
// hooks fire on the events that matter — a failed deploy, an app that fell over,
// a burst of 5xx — and fan each one out to the team's channels (email over SMTP,
// a Slack incoming webhook, a generic JSON webhook). Best-effort and async: a
// channel that errors is logged, never blocks the event.

// AlertConfig is a team's notification channels; an empty field is off.
type AlertConfig struct {
	Email      string `json:"email"`
	SlackURL   string `json:"slackUrl"`
	WebhookURL string `json:"webhookUrl"`
}

func getAlerts(team string) AlertConfig {
	var a AlertConfig
	_ = sqlDB.QueryRow(`SELECT email,slack_url,webhook_url FROM alerts WHERE team=?`, team).
		Scan(&a.Email, &a.SlackURL, &a.WebhookURL)
	return a
}

func setAlerts(team string, a AlertConfig) error {
	_, err := sqlDB.Exec(`
		INSERT INTO alerts(team,email,slack_url,webhook_url) VALUES(?,?,?,?)
		ON CONFLICT(team) DO UPDATE SET
			email=excluded.email, slack_url=excluded.slack_url, webhook_url=excluded.webhook_url`,
		team, a.Email, a.SlackURL, a.WebhookURL)
	return err
}

// alertEvent is one notifiable thing that happened.
type alertEvent struct {
	Team   string
	Kind   string // deploy.failed | app.unhealthy | app.recovered | error.spike | test
	App    string
	Title  string
	Detail string
}

type channelResult struct {
	Channel string `json:"channel"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// deliver sends an event to every configured channel and reports per-channel
// outcomes (used by the test endpoint; dispatchAlert discards them).
func deliver(ev alertEvent) []channelResult {
	cfg := getAlerts(ev.Team)
	var res []channelResult
	try := func(name string, f func() error) {
		if err := f(); err != nil {
			res = append(res, channelResult{name, false, err.Error()})
		} else {
			res = append(res, channelResult{name, true, ""})
		}
	}
	if cfg.Email != "" {
		try("email", func() error { return sendEmail(cfg.Email, ev) })
	}
	if cfg.SlackURL != "" {
		try("slack", func() error { return postSlack(cfg.SlackURL, ev) })
	}
	if cfg.WebhookURL != "" {
		try("webhook", func() error { return postWebhook(cfg.WebhookURL, ev) })
	}
	return res
}

// dispatchAlert fans an event out to the team's channels, logging failures. Call
// it as `go dispatchAlert(...)` from event sites so nothing blocks.
func dispatchAlert(ev alertEvent) {
	for _, r := range deliver(ev) {
		if !r.OK {
			log.Printf("alert %s via %s: %s", ev.Kind, r.Channel, r.Error)
		}
	}
}

// ---- channels ----

func smtpConfig() (host, addr, user, pass, from string, ok bool) {
	host = os.Getenv("SKIFF_SMTP_HOST")
	from = os.Getenv("SKIFF_SMTP_FROM")
	if host == "" || from == "" {
		return "", "", "", "", "", false
	}
	port := os.Getenv("SKIFF_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return host, host + ":" + port, os.Getenv("SKIFF_SMTP_USER"), os.Getenv("SKIFF_SMTP_PASS"), from, true
}

func sendEmail(to string, ev alertEvent) error {
	host, addr, user, pass, from, ok := smtpConfig()
	if !ok {
		return fmt.Errorf("email needs SMTP configured on the server (SKIFF_SMTP_*)")
	}
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: Skiff <%s>\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: [Skiff] %s\r\n", ev.Title)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(ev.Detail + "\r\n")
	return smtp.SendMail(addr, auth, from, []string{to}, b.Bytes())
}

func postSlack(url string, ev alertEvent) error {
	return postJSON(url, map[string]string{"text": slackText(ev)})
}

func postWebhook(url string, ev alertEvent) error {
	return postJSON(url, map[string]any{
		"kind": ev.Kind, "app": ev.App, "team": ev.Team,
		"title": ev.Title, "detail": ev.Detail, "ts": nowUnix(),
	})
}

// alertHTTPClient delivers alerts to user-supplied Slack/webhook URLs but refuses
// to connect to private, loopback, or link-local addresses. The check runs at
// dial time on the resolved IP, so it also stops DNS-rebinding and internal
// redirects from turning an alert webhook into an SSRF against the box's own
// metadata endpoint or internal services.
var alertHTTPClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if ip := net.ParseIP(host); isBlockedDialIP(ip) {
					return fmt.Errorf("refusing to connect to non-public address %s", host)
				}
				return nil
			},
		}).DialContext,
	},
}

// isBlockedDialIP is true for addresses an outbound alert must never reach.
func isBlockedDialIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}

func postJSON(url string, payload any) error {
	body, _ := json.Marshal(payload)
	resp, err := alertHTTPClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return nil
}

func slackText(ev alertEvent) string {
	icon := map[string]string{
		"deploy.failed": "🚨", "app.unhealthy": "🔴", "app.recovered": "✅",
		"error.spike": "⚠️", "test": "🔔",
	}[ev.Kind]
	if icon == "" {
		icon = "🔔"
	}
	s := icon + " *" + ev.Title + "*"
	if ev.Detail != "" {
		s += "\n" + ev.Detail
	}
	return s
}

func nowUnix() int64 { return time.Now().Unix() }

// ---- HTTP handlers ----

func (p *Panel) handleAlerts(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	switch r.Method {
	case http.MethodGet:
		cfg := getAlerts(team)
		_, _, _, _, _, smtpOK := smtpConfig()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			AlertConfig
			SMTP bool `json:"smtp"`
		}{cfg, smtpOK})
	case http.MethodPut:
		var body AlertConfig
		_ = json.NewDecoder(r.Body).Decode(&body)
		cfg := AlertConfig{
			Email:      strings.TrimSpace(body.Email),
			SlackURL:   strings.TrimSpace(body.SlackURL),
			WebhookURL: strings.TrimSpace(body.WebhookURL),
		}
		if err := setAlerts(team, cfg); err != nil {
			http.Error(w, "couldn't save", http.StatusInternalServerError)
			return
		}
		p.audit(r, "alerts.update", "", "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Panel) handleAlertTest(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	results := deliver(alertEvent{
		Team: team, Kind: "test",
		Title:  "Test alert from Skiff",
		Detail: "If you're reading this, your alert channel is wired up correctly.",
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

// ---- triggers: health + error-rate control loop ----

var (
	healthDown    = map[string]int{}   // consecutive unhealthy checks per app
	healthAlerted = map[string]bool{}  // already alerted, waiting for recovery
	spikeLast     = map[string]int64{} // last 5xx-spike alert per app (cooldown)
)

func isInflight(app string) bool {
	inflightMu.Lock()
	defer inflightMu.Unlock()
	_, ok := inflight[app]
	return ok
}

func (p *Panel) alertLoop() {
	time.Sleep(60 * time.Second) // let apps settle after a restart
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	for range tick.C {
		guard("alertLoop", func() {
			p.checkHealth()
			p.checkErrorSpikes()
		})
	}
}

// checkHealth alerts when an app has had no running container for two straight
// checks (so a deploy's brief swap doesn't page anyone), and once on recovery.
func (p *Panel) checkHealth() {
	states, err := p.eng.AppStates()
	if err != nil {
		return
	}
	apps, err := registry.Load()
	if err != nil {
		return
	}
	for name := range apps {
		src, ok := getSource(name)
		if !ok {
			continue
		}
		if isInflight(name) { // mid-deploy — not a real outage
			healthDown[name] = 0
			continue
		}
		if states[name] == "running" {
			if healthAlerted[name] {
				go dispatchAlert(alertEvent{Team: src.Team, Kind: "app.recovered", App: name,
					Title: "Recovered: " + name, Detail: "The app has a running container again."})
			}
			healthDown[name] = 0
			healthAlerted[name] = false
			continue
		}
		healthDown[name]++
		if healthDown[name] >= 2 && !healthAlerted[name] {
			healthAlerted[name] = true
			st := states[name]
			if st == "" {
				st = "missing"
			}
			go dispatchAlert(alertEvent{Team: src.Team, Kind: "app.unhealthy", App: name,
				Title:  "App down: " + name,
				Detail: "No running container (state: " + st + ")."})
		}
	}
}

// checkErrorSpikes alerts when an app serves a burst of 5xx over the last 5
// minutes, with enough traffic to be meaningful and a per-app cooldown.
func (p *Panel) checkErrorSpikes() {
	data := readMetricsFile()
	now := time.Now().Unix()
	cutoff := now - 5*60
	for app, buckets := range data.Apps {
		src, ok := getSource(app)
		if !ok {
			continue
		}
		var s5, req int
		for _, b := range buckets {
			if b.T >= cutoff {
				s5 += b.S5
				req += b.Req
			}
		}
		if req < 20 || s5 < 10 {
			continue // too little traffic, or too few errors, to judge
		}
		rate := float64(s5) / float64(req)
		if rate < 0.10 {
			continue
		}
		if now-spikeLast[app] < 15*60 {
			continue // cooldown
		}
		spikeLast[app] = now
		go dispatchAlert(alertEvent{Team: src.Team, Kind: "error.spike", App: app,
			Title:  "5xx spike: " + app,
			Detail: fmt.Sprintf("%d server errors in 5 min — %.0f%% of %d requests.", s5, rate*100, req)})
	}
}
