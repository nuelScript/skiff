package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Access tokens put a key in front of an app: once it has one, the router lets through only requests that carry one, as an MCP client or another service would. Previews follow their project.
const maxAccessTokens = 20

type accessToken struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
	Token   string `json:"token,omitempty"` // only in the create response
}

func accessTokenHashes(app string) []string {
	rows, err := sqlDB.Query(`SELECT hash FROM access_tokens WHERE app=? ORDER BY created`, app)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if rows.Scan(&h) == nil {
			out = append(out, h)
		}
	}
	return out
}

func listAccessTokens(app string) []accessToken {
	rows, err := sqlDB.Query(`SELECT id, name, created FROM access_tokens WHERE app=? ORDER BY created`, app)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []accessToken{}
	for rows.Next() {
		var t accessToken
		if rows.Scan(&t.ID, &t.Name, &t.Created) == nil {
			out = append(out, t)
		}
	}
	return out
}

func createAccessToken(app, name string, now time.Time) (accessToken, error) {
	t := accessToken{ID: randToken()[:12], Name: name, Created: now.Unix(), Token: "skat_" + randHex(24)}
	_, err := sqlDB.Exec(`INSERT INTO access_tokens(id, app, name, hash, created) VALUES(?,?,?,?,?)`, t.ID, app, name, hashToken(t.Token), t.Created)
	return t, err
}

func (p *Panel) handleAccessTokens(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if src.Parent != "" && r.Method != http.MethodGet {
		http.Error(w, "previews use their project's access tokens", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"tokens": listAccessTokens(app)})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" || len(name) > 60 {
			http.Error(w, "name the token, like claude-desktop, in up to 60 characters", http.StatusBadRequest)
			return
		}
		if len(listAccessTokens(app)) >= maxAccessTokens {
			http.Error(w, "an app can have 20 access tokens; revoke one first", http.StatusBadRequest)
			return
		}
		t, err := createAccessToken(app, name, time.Now())
		if err != nil {
			http.Error(w, "couldn't create the token", http.StatusInternalServerError)
			return
		}
		_ = writeEdgeFile()
		p.audit(r, "access-token.create", app, name)
		writeJSON(w, t)
	case http.MethodDelete:
		id := sanitizeID(r.URL.Query().Get("id"))
		if _, err := sqlDB.Exec(`DELETE FROM access_tokens WHERE app=? AND id=?`, app, id); err != nil {
			http.Error(w, "couldn't revoke", http.StatusInternalServerError)
			return
		}
		_ = writeEdgeFile()
		p.audit(r, "access-token.revoke", app, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
