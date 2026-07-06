package panel

import (
	"encoding/json"
	"net/http"

	"github.com/nuelScript/skiff/internal/auth"
)

// handleAccount updates the signed-in user's profile (display name).
func (p *Panel) handleAccount(w http.ResponseWriter, r *http.Request) {
	s, ok := p.session(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct{ Name string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := p.auth.UpdateName(s.userID, body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePassword changes the signed-in user's password after verifying the current one.
func (p *Panel) handlePassword(w http.ResponseWriter, r *http.Request) {
	s, ok := p.session(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct{ Current, Password string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := p.auth.ChangePassword(s.userID, body.Current, body.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func countTeamApps(team string) int {
	var n int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM sources WHERE team = ?`, team).Scan(&n)
	return n
}

func countTeamDatabases(team string) int {
	var n int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM databases WHERE team = ?`, team).Scan(&n)
	return n
}

// reassignTeam moves the caller's session onto another team they belong to (or
// none), used after they leave or delete the current one.
func (p *Panel) reassignTeam(w http.ResponseWriter, r *http.Request, userID string) {
	next := ""
	if teams := p.auth.TeamsForUser(userID); len(teams) > 0 {
		next = teams[0].ID
	}
	if c, err := r.Cookie("skiff_session"); err == nil {
		setSessionTeam(c.Value, next)
	}
}

// handleTeamLeave removes the caller from the current team.
func (p *Panel) handleTeamLeave(w http.ResponseWriter, r *http.Request) {
	s, ok := p.session(r)
	if !ok || r.Method != http.MethodPost {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if len(p.auth.TeamsForUser(s.userID)) <= 1 {
		http.Error(w, "you can't leave your only team", http.StatusBadRequest)
		return
	}
	if err := p.auth.LeaveTeam(s.userID, s.teamID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.reassignTeam(w, r, s.userID)
	w.WriteHeader(http.StatusNoContent)
}

// handleTeamDelete deletes the current team (owners only) once it holds no apps
// or databases.
func (p *Panel) handleTeamDelete(w http.ResponseWriter, r *http.Request) {
	s, ok := p.session(r)
	if !ok || r.Method != http.MethodPost {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if role, ok := p.auth.Role(s.userID, s.teamID); !ok || role != auth.RoleOwner {
		http.Error(w, "only owners can delete a team", http.StatusForbidden)
		return
	}
	if len(p.auth.TeamsForUser(s.userID)) <= 1 {
		http.Error(w, "you can't delete your only team", http.StatusBadRequest)
		return
	}
	if countTeamApps(s.teamID) > 0 {
		http.Error(w, "remove this team's projects first", http.StatusBadRequest)
		return
	}
	if countTeamDatabases(s.teamID) > 0 {
		http.Error(w, "remove this team's databases first", http.StatusBadRequest)
		return
	}
	_, _ = sqlDB.Exec(`DELETE FROM shared_env WHERE team = ?`, s.teamID)
	if err := p.auth.DeleteTeam(s.teamID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.reassignTeam(w, r, s.userID)
	w.WriteHeader(http.StatusNoContent)
}

// handleTeamRename renames the current team (owners only).
func (p *Panel) handleTeamRename(w http.ResponseWriter, r *http.Request) {
	s, ok := p.session(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if role, ok := p.auth.Role(s.userID, s.teamID); !ok || role != auth.RoleOwner {
		http.Error(w, "only owners can rename the team", http.StatusForbidden)
		return
	}
	var body struct{ Name string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := p.auth.RenameTeam(s.teamID, body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
