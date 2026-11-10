package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
)

// API tokens authenticate the /api/v1 surface for CI; each is team-scoped, shown once at creation, and stored only as its SHA-256.

type APIToken struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Created  int64  `json:"created"`
	LastUsed int64  `json:"lastUsed"`
	// Token is set only in the create response — never persisted or listed.
	Token string `json:"token,omitempty"`
	// Scopes and Apps limit what it can do and to which projects; empty means everything.
	Scopes []string `json:"scopes,omitempty"`
	Apps   []string `json:"apps,omitempty"`
	// Mine is set in a listing when the caller created it.
	Mine bool `json:"mine,omitempty"`
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func newAPIToken() string {
	return "skiff_" + randHex(24)
}

func createToken(team, name, createdBy string, grant tokenGrant) (APIToken, error) {
	tok := newAPIToken()
	t := APIToken{ID: randToken()[:12], Name: name, Created: time.Now().Unix(), Scopes: grant.Scopes, Apps: grant.Apps}
	_, err := sqlDB.Exec(
		`INSERT INTO api_tokens(id,team,name,token_hash,created,last_used,created_by,scopes,apps) VALUES(?,?,?,?,?,0,?,?,?)`,
		t.ID, team, name, hashToken(tok), t.Created, createdBy, encodeList(grant.Scopes), encodeList(grant.Apps))
	if err != nil {
		return APIToken{}, err
	}
	t.Token = tok
	return t, nil
}

func allowDatabases(id string) error {
	_, err := sqlDB.Exec(`UPDATE api_tokens SET reach_databases=1 WHERE id=?`, id)
	return err
}

func tokenReachesDatabases(tok string) bool {
	var allowed int
	err := sqlDB.QueryRow(`SELECT reach_databases FROM api_tokens WHERE token_hash=?`, hashToken(tok)).Scan(&allowed)
	return err == nil && allowed == 1
}

func listTokens(team, userID string) []APIToken {
	rows, err := sqlDB.Query(
		`SELECT id,name,created,last_used,scopes,apps,COALESCE(created_by,'') FROM api_tokens WHERE team=? ORDER BY created DESC`, team)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		var scopes, apps, by string
		if rows.Scan(&t.ID, &t.Name, &t.Created, &t.LastUsed, &scopes, &apps, &by) == nil {
			t.Scopes, t.Apps, t.Mine = decodeList(scopes), decodeList(apps), by == userID
			out = append(out, t)
		}
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}

func revokeToken(team, id string) {
	_, _ = sqlDB.Exec(`DELETE FROM api_tokens WHERE team=? AND id=?`, team, id)
}

func resolveToken(tok string) (team, name string, grant tokenGrant, ok bool) {
	if tok == "" {
		return "", "", tokenGrant{}, false
	}
	var id, scopes, apps string
	err := sqlDB.QueryRow(
		`SELECT id,team,name,scopes,apps FROM api_tokens WHERE token_hash=?`, hashToken(tok)).
		Scan(&id, &team, &name, &scopes, &apps)
	if err != nil {
		return "", "", tokenGrant{}, false
	}
	_, _ = sqlDB.Exec(`UPDATE api_tokens SET last_used=? WHERE id=?`, time.Now().Unix(), id)
	return team, name, tokenGrant{Scopes: decodeList(scopes), Apps: decodeList(apps)}, true
}

func (p *Panel) handleTokens(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	if team == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s, _ := p.session(r)
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, listTokens(team, s.userID))
	case http.MethodPost:
		var body struct {
			Name      string
			Databases bool
			Scopes    []string
			Apps      []string
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Databases && !p.isOwner(r) {
			http.Error(w, "only a team owner can create a token that reaches databases", http.StatusForbidden)
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = "token"
		}
		grant, problem := checkGrant(team, body.Scopes, body.Apps)
		if problem != "" {
			http.Error(w, problem, http.StatusBadRequest)
			return
		}
		t, err := createToken(team, name, s.userID, grant)
		if err != nil {
			http.Error(w, "could not create token", http.StatusInternalServerError)
			return
		}
		if body.Databases {
			if err := allowDatabases(t.ID); err != nil {
				revokeToken(team, t.ID)
				http.Error(w, "could not create token", http.StatusInternalServerError)
				return
			}
		}
		p.audit(r, "token.create", name, "")
		writeJSON(w, t) // includes the plaintext token, this one time
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		var revoked, by string
		_ = sqlDB.QueryRow(`SELECT name,COALESCE(created_by,'') FROM api_tokens WHERE team=? AND id=?`, team, id).Scan(&revoked, &by)
		if by != s.userID && !p.isOwner(r) {
			http.Error(w, "only a team owner can revoke a token someone else created", http.StatusForbidden)
			return
		}
		revokeToken(team, id)
		p.audit(r, "token.revoke", revoked, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// checkGrant keeps a new token's scopes to known ones and its projects to the team's own.
func checkGrant(team string, scopes, apps []string) (tokenGrant, string) {
	var g tokenGrant
	for _, sc := range scopes {
		if !slices.Contains(allScopes, sc) {
			return g, "unknown permission " + sc
		}
		if !slices.Contains(g.Scopes, sc) {
			g.Scopes = append(g.Scopes, sc)
		}
	}
	if len(g.Scopes) == len(allScopes) {
		g.Scopes = nil
	}
	for _, a := range apps {
		src, ok := getSource(sanitizeName(a))
		if !ok || src.Team != team || src.Parent != "" {
			return g, "your team has no project named " + a
		}
		if !slices.Contains(g.Apps, src.App) {
			g.Apps = append(g.Apps, src.App)
		}
	}
	return g, ""
}
