package panel

import (
	"encoding/json"
	"net/http"
	"time"
)

// The audit log is a per-team, append-only trail of who did what: every deploy,
// rollback, secret change, membership change, and teardown, attributed to the
// person (or "push", for a webhook) who triggered it. Recording is fire-and-
// forget at each mutation's success point; the Activity page reads it back.

type AuditEntry struct {
	ID      int64  `json:"id"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	Detail  string `json:"detail"`
	Created int64  `json:"created"`
}

// recordAudit appends an entry attributed to a named actor (use for system /
// webhook actions that have no session, e.g. actor "push").
func recordAudit(team, actor, action, target, detail string) {
	if team == "" || sqlDB == nil {
		return
	}
	_, _ = sqlDB.Exec(
		`INSERT INTO audit(team,actor,action,target,detail,created) VALUES(?,?,?,?,?,?)`,
		team, actor, action, target, detail, time.Now().Unix())
}

// audit appends an entry attributed to the request's signed-in user.
func (p *Panel) audit(r *http.Request, action, target, detail string) {
	s, ok := p.session(r)
	if !ok {
		return
	}
	actor := "someone"
	if u, ok := p.auth.User(s.userID); ok {
		if actor = u.Email; actor == "" {
			actor = u.Name
		}
	}
	recordAudit(s.teamID, actor, action, target, detail)
}

func teamAudit(team string, limit int) []AuditEntry {
	rows, err := sqlDB.Query(
		`SELECT id,actor,action,target,detail,created FROM audit WHERE team=? ORDER BY id DESC LIMIT ?`,
		team, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.Created) == nil {
			out = append(out, e)
		}
	}
	return out
}

func (p *Panel) handleAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(teamAudit(p.teamID(r), 200))
}
