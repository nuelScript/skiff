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
