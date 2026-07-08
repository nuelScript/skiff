package panel

import (
	"encoding/json"
	"net/http"
	"time"
)

type AuditEntry struct {
	ID      int64  `json:"id"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	Detail  string `json:"detail"`
	Created int64  `json:"created"`
}

func recordAudit(team string, e AuditEntry) {
	if team == "" || sqlDB == nil {
		return
	}
	_, _ = sqlDB.Exec(
		`INSERT INTO audit(team,actor,action,target,detail,created) VALUES(?,?,?,?,?,?)`,
		team, e.Actor, e.Action, e.Target, e.Detail, time.Now().Unix())
}

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
	recordAudit(s.teamID, AuditEntry{Actor: actor, Action: action, Target: target, Detail: detail})
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
	if rows.Err() != nil {
		return nil
	}
	return out
}

func (p *Panel) handleAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(teamAudit(p.teamID(r), 200))
}
