package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// An approval is a risky action an agent asks a person about first. The agent waits on the answer; the team hears about the request through its alerts and answers on the agent's page.
const (
	approvalExpire     = 24 * time.Hour
	approvalMaxWait    = 60 * time.Second
	approvalMaxPending = 20
	approvalKeep       = 100
	approvalMaxText    = 4 << 10
)

const (
	approvalPending  = "pending"
	approvalApproved = "approved"
	approvalDenied   = "denied"
	approvalExpired  = "expired"
)

type approval struct {
	ID       int64  `json:"id"`
	Action   string `json:"action"`
	Detail   string `json:"detail"`
	Status   string `json:"status"`
	Note     string `json:"note,omitempty"`
	By       string `json:"by,omitempty"`
	Created  int64  `json:"created"`
	Answered int64  `json:"answered,omitempty"`
}

var errTooManyApprovals = errors.New("this agent already has 20 requests waiting for an answer")

func askApproval(app, team, action, detail string, now time.Time) (approval, error) {
	var pending int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM approvals WHERE app=? AND status=?`, app, approvalPending).Scan(&pending)
	if pending >= approvalMaxPending {
		return approval{}, errTooManyApprovals
	}
	res, err := sqlDB.Exec(`INSERT INTO approvals(app, action, detail, status, created) VALUES(?,?,?,?,?)`, app, action, detail, approvalPending, now.Unix())
	if err != nil {
		return approval{}, err
	}
	id, _ := res.LastInsertId()
	_, _ = sqlDB.Exec(`DELETE FROM approvals WHERE app=? AND status<>? AND id NOT IN (SELECT id FROM approvals WHERE app=? ORDER BY id DESC LIMIT ?)`,
		app, approvalPending, app, approvalKeep)
	go dispatchAlert(alertEvent{Team: team, Kind: "approval.requested", App: app,
		Title:  app + " is asking to " + action,
		Detail: strings.TrimSpace(detail + "\n\nApprove or deny it on " + app + "'s page: " + dashboardURL + "/projects/" + app)})
	return approval{ID: id, Action: action, Detail: detail, Status: approvalPending, Created: now.Unix()}, nil
}

// getApproval reads a request, expiring it first if nobody answered in time.
func getApproval(app string, id int64, now time.Time) (approval, bool) {
	_, _ = sqlDB.Exec(`UPDATE approvals SET status=?, answered=? WHERE app=? AND status=? AND created<?`,
		approvalExpired, now.Unix(), app, approvalPending, now.Add(-approvalExpire).Unix())
	var a approval
	err := sqlDB.QueryRow(`SELECT id, action, detail, status, note, answered_by, created, answered FROM approvals WHERE app=? AND id=?`, app, id).
		Scan(&a.ID, &a.Action, &a.Detail, &a.Status, &a.Note, &a.By, &a.Created, &a.Answered)
	return a, err == nil
}

func listApprovals(app string, now time.Time, limit int) []approval {
	_, _ = sqlDB.Exec(`UPDATE approvals SET status=?, answered=? WHERE app=? AND status=? AND created<?`,
		approvalExpired, now.Unix(), app, approvalPending, now.Add(-approvalExpire).Unix())
	rows, err := sqlDB.Query(`SELECT id, action, detail, status, note, answered_by, created, answered FROM approvals WHERE app=?
		ORDER BY status=? DESC, id DESC LIMIT ?`, app, approvalPending, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []approval{}
	for rows.Next() {
		var a approval
		if rows.Scan(&a.ID, &a.Action, &a.Detail, &a.Status, &a.Note, &a.By, &a.Created, &a.Answered) == nil {
			out = append(out, a)
		}
	}
	return out
}

// answerApproval records a person's decision; only a request still waiting can be answered.
func answerApproval(app string, id int64, approve bool, note, by string, now time.Time) error {
	status := approvalDenied
	if approve {
		status = approvalApproved
	}
	if a, ok := getApproval(app, id, now); !ok {
		return errors.New("no such request")
	} else if a.Status != approvalPending {
		return fmt.Errorf("this request was already %s", a.Status)
	}
	_, err := sqlDB.Exec(`UPDATE approvals SET status=?, note=?, answered_by=?, answered=? WHERE app=? AND id=? AND status=?`,
		status, note, by, now.Unix(), app, id, approvalPending)
	return err
}

// waitApproval answers with the request once it's decided, or as it stands when the wait runs out.
func waitApproval(w http.ResponseWriter, r *http.Request, app string, id int64) {
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	deadline := time.Now().Add(min(time.Duration(max(wait, 0))*time.Second, approvalMaxWait))
	for {
		a, ok := getApproval(app, id, time.Now())
		if !ok {
			gatewayError(w, http.StatusNotFound, "no such request")
			return
		}
		if a.Status != approvalPending || time.Now().After(deadline) {
			writeJSON(w, a)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// agentApprovals is the gateway side: POST approvals asks, GET approvals/<id> waits for the answer.
func agentApprovals(w http.ResponseWriter, r *http.Request, a aiApp, path string) {
	rest := strings.TrimPrefix(strings.TrimPrefix(path, "approvals"), "/")
	switch {
	case rest == "" && r.Method == http.MethodPost:
		var body struct {
			Action string `json:"action"`
			Detail string `json:"detail"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*approvalMaxText)).Decode(&body) != nil {
			gatewayError(w, http.StatusBadRequest, `send JSON like {"action": "delete 40 rows from orders", "detail": "..."}`)
			return
		}
		body.Action, body.Detail = strings.TrimSpace(body.Action), strings.TrimSpace(body.Detail)
		if body.Action == "" || len(body.Action) > 200 || len(body.Detail) > approvalMaxText {
			gatewayError(w, http.StatusBadRequest, "say what the action is in up to 200 characters, with up to 4 KB of detail")
			return
		}
		req, err := askApproval(a.App, a.Team, body.Action, body.Detail, time.Now())
		if errors.Is(err, errTooManyApprovals) {
			gatewayError(w, http.StatusTooManyRequests, err.Error())
			return
		}
		if err != nil {
			gatewayError(w, http.StatusInternalServerError, "couldn't record the request")
			return
		}
		if r.URL.Query().Get("wait") != "" {
			waitApproval(w, r, a.App, req.ID)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(req)
	case rest != "" && r.Method == http.MethodGet:
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			gatewayError(w, http.StatusNotFound, "no such request")
			return
		}
		waitApproval(w, r, a.App, id)
	default:
		gatewayError(w, http.StatusNotFound, "approvals have POST /_skiff/approvals and GET /_skiff/approvals/<id>")
	}
}

// handleApprovals lets the team see and answer an agent's requests.
func (p *Panel) handleApprovals(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	if _, ok := getSource(app); !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"approvals": listApprovals(app, time.Now(), 20)})
	case http.MethodPost:
		var body struct {
			ID      int64  `json:"id"`
			Approve bool   `json:"approve"`
			Note    string `json:"note"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, approvalMaxText)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		by := p.deployActor(r)
		if err := answerApproval(app, body.ID, body.Approve, strings.TrimSpace(body.Note), by, time.Now()); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		verdict := "deny"
		if body.Approve {
			verdict = "approve"
		}
		p.audit(r, "agent.approval."+verdict, app, strconv.FormatInt(body.ID, 10))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func denyPendingApprovals(app, note string, now time.Time) {
	_, _ = sqlDB.Exec(`UPDATE approvals SET status=?, note=?, answered=? WHERE app=? AND status=?`, approvalDenied, note, now.Unix(), app, approvalPending)
}
