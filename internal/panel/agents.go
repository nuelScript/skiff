package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/seal"
)

// An agent is a long-running project with no public address. It keeps its state on a volume, calls models through the AI gateway, and takes its work from an inbox that webhooks, GitHub and a schedule fill, so it needs no open port and loses nothing when it crashes.
const (
	agentStatePath = "/data"
	agentStateName = "state"
	// agentBudget is a new agent's monthly token budget, so a loop can't spend without limit before anyone sets one.
	agentBudget = 1_000_000
	// agentLease is how long an agent has to finish an event before it's handed out again, as after a crash.
	agentLease      = 5 * time.Minute
	agentKeepEvents = 7 * 24 * time.Hour
	agentMaxPending = 1000
	agentMaxWait    = 60 * time.Second
	agentTickEvery  = 30 * time.Second
)

var errInboxFull = errors.New("the agent has 1000 events waiting; it isn't taking them")

type agentRow struct {
	App, Team, Schedule string
	hookSecret          string
	LastTick            int64
	Stopped             bool
	Created             int64
}

func getAgent(app string) (agentRow, bool) {
	var a agentRow
	var stopped int
	err := sqlDB.QueryRow(`SELECT app, team, hook_secret, schedule, last_tick, stopped, created FROM agents WHERE app=?`, app).
		Scan(&a.App, &a.Team, &a.hookSecret, &a.Schedule, &a.LastTick, &stopped, &a.Created)
	if err != nil || seal.OpenAll(&a.hookSecret) != nil {
		return agentRow{}, false
	}
	a.Stopped = stopped != 0
	return a, true
}

// applyAgentPreset makes a new project an agent before its first deploy: nothing routes to it and nothing checks a port.
func applyAgentPreset(src *Source) {
	src.Private = true
	src.HealthMode = "none"
}

// setupAgent gives a new agent its inbox, a volume for its state, and a gateway token with a starting budget.
func setupAgent(src Source) error {
	secret, err := seal.Seal(randHex(24))
	if err != nil {
		return err
	}
	if _, err := sqlDB.Exec(`INSERT INTO agents(app, team, hook_secret, created) VALUES(?,?,?,?) ON CONFLICT(app) DO NOTHING`,
		src.App, src.Team, secret, time.Now().Unix()); err != nil {
		return err
	}
	has := false
	for _, v := range listVolumes(src.App) {
		has = has || v.Name == agentStateName
	}
	if !has {
		if err := addVolume(src.App, config.Volume{Name: agentStateName, Path: agentStatePath}); err != nil {
			return err
		}
	}
	if _, on := aiAppFor(src.App); !on {
		return setAIApp(src, true, agentBudget)
	}
	return nil
}

type agentEvent struct {
	ID         int64  `json:"id"`
	Source     string `json:"source"`
	Kind       string `json:"kind"`
	Body       string `json:"body"`
	Created    int64  `json:"created"`
	Deliveries int    `json:"deliveries"`
	Done       bool   `json:"done"`
}

func addAgentEvent(app, source, kind, body string, now time.Time) (int64, error) {
	var pending int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM agent_events WHERE app=? AND done=0`, app).Scan(&pending)
	if pending >= agentMaxPending {
		return 0, errInboxFull
	}
	res, err := sqlDB.Exec(`INSERT INTO agent_events(app, source, kind, body, created) VALUES(?,?,?,?,?)`, app, source, kind, body, now.Unix())
	if err != nil {
		return 0, err
	}
	_, _ = sqlDB.Exec(`DELETE FROM agent_events WHERE app=? AND done=1 AND created<?`, app, now.Add(-agentKeepEvents).Unix())
	return res.LastInsertId()
}

// takeAgentEvent hands out the oldest event nobody is working on, and leases it so a crash puts it back.
func takeAgentEvent(app string, now time.Time) (agentEvent, bool) {
	tx, err := sqlDB.Begin()
	if err != nil {
		return agentEvent{}, false
	}
	defer tx.Rollback() //nolint:errcheck
	var e agentEvent
	err = tx.QueryRow(`SELECT id, source, kind, body, created, deliveries FROM agent_events WHERE app=? AND done=0 AND leased_until<=? ORDER BY id LIMIT 1`,
		app, now.Unix()).Scan(&e.ID, &e.Source, &e.Kind, &e.Body, &e.Created, &e.Deliveries)
	if err != nil {
		return agentEvent{}, false
	}
	e.Deliveries++
	if _, err := tx.Exec(`UPDATE agent_events SET leased_until=?, deliveries=? WHERE id=?`, now.Add(agentLease).Unix(), e.Deliveries, e.ID); err != nil {
		return agentEvent{}, false
	}
	return e, tx.Commit() == nil
}

func finishAgentEvent(app string, id int64) bool {
	res, err := sqlDB.Exec(`UPDATE agent_events SET done=1 WHERE app=? AND id=?`, app, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func recentAgentEvents(app string, limit int) []agentEvent {
	rows, err := sqlDB.Query(`SELECT id, source, kind, substr(body, 1, 300), created, deliveries, done FROM agent_events WHERE app=? ORDER BY id DESC LIMIT ?`, app, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []agentEvent{}
	for rows.Next() {
		var e agentEvent
		var done int
		if rows.Scan(&e.ID, &e.Source, &e.Kind, &e.Body, &e.Created, &e.Deliveries, &done) == nil {
			e.Done = done != 0
			out = append(out, e)
		}
	}
	return out
}

// agentAPI is what an agent reaches on the gateway besides models: GET events waits for the next event, POST events/<id>/done finishes it, and POST sandboxes runs code in a throwaway container.
func (p *Panel) agentAPI(w http.ResponseWriter, r *http.Request, a aiApp, path string) {
	if _, ok := getAgent(a.App); !ok {
		gatewayError(w, http.StatusNotFound, a.App+" isn't an agent; only agents have an inbox and sandboxes")
		return
	}
	switch {
	case path == otlpPath:
		handleOTLP(w, r, a)
	case path == "sandboxes" && r.Method == http.MethodPost:
		p.runSandbox(w, r, a)
	case path == "events" && r.Method == http.MethodGet:
		wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
		deadline := time.Now().Add(min(time.Duration(max(wait, 0))*time.Second, agentMaxWait))
		for {
			if e, ok := takeAgentEvent(a.App, time.Now()); ok {
				writeJSON(w, e)
				return
			}
			if time.Now().After(deadline) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
	case strings.HasPrefix(path, "events/") && strings.HasSuffix(path, "/done") && r.Method == http.MethodPost:
		id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(path, "events/"), "/done"), 10, 64)
		if err != nil || !finishAgentEvent(a.App, id) {
			gatewayError(w, http.StatusNotFound, "no such event")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		gatewayError(w, http.StatusNotFound, "agents have GET /_skiff/events, POST /_skiff/events/<id>/done and POST /_skiff/sandboxes")
	}
}

// handleAgentHook puts a signed request in the agent's inbox. A GitHub webhook pointed here arrives as its event.
func (p *Panel) handleAgentHook(w http.ResponseWriter, r *http.Request) {
	a, ok := getAgent(sanitizeName(r.PathValue("app")))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBodyLimit))
	if err != nil {
		http.Error(w, "the body is over 64 KB", http.StatusRequestEntityTooLarge)
		return
	}
	if !signedBy(r, body, a.hookSecret) {
		http.Error(w, "the signature doesn't match", http.StatusUnauthorized)
		return
	}
	source, kind := "webhook", r.Header.Get("X-Skiff-Event")
	if gh := r.Header.Get("X-GitHub-Event"); gh != "" {
		if gh == "ping" {
			writeJSON(w, map[string]string{"ok": "pong"})
			return
		}
		source, kind = "github", gh
	}
	id, err := addAgentEvent(a.App, source, kind, string(body), time.Now())
	if errors.Is(err, errInboxFull) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if err != nil {
		http.Error(w, "couldn't take the event", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]int64{"event": id})
}

// agentLoop puts a scheduled event in each agent's inbox when its time comes. Times missed while Skiff was down become one event, not a burst.
func (p *Panel) agentLoop() {
	for {
		time.Sleep(agentTickEvery)
		tickAgents(time.Now())
	}
}

func tickAgents(now time.Time) {
	rows, err := sqlDB.Query(`SELECT app, schedule, last_tick, created FROM agents WHERE schedule<>'' AND stopped=0`)
	if err != nil {
		return
	}
	type due struct {
		app, schedule string
	}
	var ready []due
	for rows.Next() {
		var app, schedule string
		var last, created int64
		if rows.Scan(&app, &schedule, &last, &created) != nil {
			continue
		}
		s, err := cron.ParseStandard(schedule)
		if err != nil {
			continue
		}
		if !s.Next(time.Unix(max(last, created), 0)).After(now) {
			ready = append(ready, due{app, schedule})
		}
	}
	rows.Close()
	for _, d := range ready {
		_, _ = sqlDB.Exec(`UPDATE agents SET last_tick=? WHERE app=?`, now.Unix(), d.app)
		_, _ = addAgentEvent(d.app, "schedule", d.schedule, "", now)
	}
}

// stopAgent stops everything the agent runs and cuts off its model access and inbox, in one step. startAgent undoes it.
func (p *Panel) stopAgent(a agentRow) error {
	if _, err := sqlDB.Exec(`UPDATE agents SET stopped=1 WHERE app=?`, a.App); err != nil {
		return err
	}
	cancelAppRuns(a.App)
	p.stopSandboxes(a.App)
	var failed []string
	for _, c := range append(p.eng.AppContainers(a.App), p.eng.WorkerContainers(a.App)...) {
		if err := p.eng.Stop(c); err != nil {
			failed = append(failed, c)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("couldn't stop %s", strings.Join(failed, ", "))
	}
	return nil
}

func (p *Panel) startAgent(a agentRow) error {
	if _, err := sqlDB.Exec(`UPDATE agents SET stopped=0 WHERE app=?`, a.App); err != nil {
		return err
	}
	for _, c := range append(p.eng.AppContainers(a.App), p.eng.WorkerContainers(a.App)...) {
		_ = p.eng.Start(c)
	}
	return nil
}

func agentStopped(app string) bool {
	var stopped int
	return sqlDB.QueryRow(`SELECT stopped FROM agents WHERE app=?`, app).Scan(&stopped) == nil && stopped != 0
}

func (p *Panel) handleProjectAgent(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	if _, ok := getSource(app); !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	a, ok := getAgent(app)
	if !ok {
		writeJSON(w, map[string]any{"agent": false})
		return
	}
	switch r.Method {
	case http.MethodGet:
		var pending int
		_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM agent_events WHERE app=? AND done=0`, app).Scan(&pending)
		var next int64
		if s, err := cron.ParseStandard(a.Schedule); err == nil && a.Schedule != "" {
			next = s.Next(time.Unix(max(a.LastTick, a.Created), 0)).Unix()
		}
		writeJSON(w, map[string]any{
			"agent": true, "stopped": a.Stopped, "schedule": a.Schedule, "next": next,
			"webhook": dashboardURL + "/api/hooks/agents/" + app, "webhookSecret": a.hookSecret,
			"pending": pending, "events": recentAgentEvents(app, 20), "statePath": agentStatePath,
			"sandboxes": recentSandboxes(app, 10),
		})
	case http.MethodPut:
		var body struct {
			Schedule string `json:"schedule"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		schedule := strings.TrimSpace(body.Schedule)
		if schedule != "" {
			if _, err := cron.ParseStandard(schedule); err != nil {
				http.Error(w, `invalid schedule: use cron syntax like "*/15 * * * *"`, http.StatusBadRequest)
				return
			}
		}
		if _, err := sqlDB.Exec(`UPDATE agents SET schedule=?, last_tick=? WHERE app=?`, schedule, time.Now().Unix(), app); err != nil {
			http.Error(w, "couldn't save", http.StatusInternalServerError)
			return
		}
		p.audit(r, "agent.schedule", app, schedule)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		var err error
		switch action := r.URL.Query().Get("action"); action {
		case "stop":
			err = p.stopAgent(a)
		case "start":
			err = p.startAgent(a)
		default:
			http.Error(w, "action must be stop or start", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.audit(r, "agent."+r.URL.Query().Get("action"), app, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
