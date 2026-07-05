// Package panel is Skiff's hosted control panel: a login-gated web app with
// accounts + teams, where teams own projects deployed from git and managed on
// the box. Served behind the router at dash.<domain>.
package panel

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nuelScript/skiff/internal/auth"
	"github.com/nuelScript/skiff/internal/db"
	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/registry"
)

type sess struct {
	userID string
	teamID string
}

type Panel struct {
	setupSecret string // gate on first-run setup, so randoms can't claim the box
	domain      string
	eng         *docker.Engine
	buildsDir   string
	auth        *auth.Store

	// selfRepo/selfBranch identify Skiff's own repository, so a push to it
	// rebuilds and hot-swaps the control plane. Empty (unset) disables self-deploy.
	selfRepo   string
	selfBranch string
}

func New(setupSecret, domain string, eng *docker.Engine) (*Panel, error) {
	database, err := db.Open()
	if err != nil {
		return nil, err
	}
	sqlDB = database
	home, _ := os.UserHomeDir()
	branch := os.Getenv("SKIFF_SELF_BRANCH")
	if branch == "" {
		branch = "main"
	}
	return &Panel{
		setupSecret: setupSecret,
		domain:      domain,
		eng:         eng,
		buildsDir:   filepath.Join(home, ".skiff", "builds"),
		auth:        auth.NewStore(database),
		selfRepo:    os.Getenv("SKIFF_SELF_REPO"),
		selfBranch:  branch,
	}, nil
}

func (p *Panel) Handler() http.Handler {
	mux := http.NewServeMux()
	// auth + account state
	mux.HandleFunc("/api/me", p.handleMe)
	mux.HandleFunc("/api/auth/setup", p.handleSetup)
	mux.HandleFunc("/api/auth/login", p.handleLogin)
	mux.HandleFunc("/api/auth/logout", p.handleLogout)
	mux.HandleFunc("/api/auth/accept", p.handleAccept)
	mux.HandleFunc("/api/auth/team", p.protected(p.handleTeamSwitch))
	mux.HandleFunc("/api/teams", p.protected(p.handleTeamCreate))
	mux.HandleFunc("/api/teams/members", p.protected(p.handleMembers))
	mux.HandleFunc("/api/teams/invite", p.protected(p.handleInvite))
	// projects
	mux.HandleFunc("/api/system", p.protected(p.handleSystem))
	mux.HandleFunc("/api/apps", p.protected(p.handleApps))
	mux.HandleFunc("/api/deploy", p.protected(p.handleDeploy))
	mux.HandleFunc("/api/logs", p.protected(p.handleLogs))
	mux.HandleFunc("/api/down", p.protected(p.handleDown))
	mux.HandleFunc("/api/env", p.protected(p.handleEnv))
	mux.HandleFunc("/api/deploys", p.protected(p.handleDeploys))
	mux.HandleFunc("/api/deploys/log", p.protected(p.handleDeployLog))
	// github
	mux.HandleFunc("/api/github/status", p.protected(p.handleGithubStatus))
	mux.HandleFunc("/api/github/create", p.protected(p.handleGithubCreate))
	mux.HandleFunc("/api/github/created", p.protected(p.handleGithubCreated))
	mux.HandleFunc("/api/github/installed", p.protected(p.handleGithubInstalled))
	mux.HandleFunc("/api/github/repos", p.protected(p.handleGithubRepos))
	mux.HandleFunc("/api/github/deploy", p.protected(p.handleGithubDeploy))
	mux.HandleFunc("/api/github/hook", p.handleHook)
	mux.Handle("/", p.spa())
	return mux
}

func (p *Panel) protected(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := p.session(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (p *Panel) session(r *http.Request) (sess, bool) {
	c, err := r.Cookie("skiff_session")
	if err != nil {
		return sess{}, false
	}
	return getSession(c.Value)
}

func (p *Panel) setSession(w http.ResponseWriter, userID, teamID string) {
	tok := randToken()
	putSession(tok, userID, teamID)
	http.SetCookie(w, &http.Cookie{Name: "skiff_session", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// teamID returns the caller's active team.
func (p *Panel) teamID(r *http.Request) string {
	s, _ := p.session(r)
	return s.teamID
}

// canAccess is true when the caller is a member of the team that owns the app.
func (p *Panel) canAccess(r *http.Request, app string) bool {
	src, ok := getSource(app)
	if !ok || src.Team == "" {
		return false
	}
	s, ok := p.session(r)
	if !ok {
		return false
	}
	_, member := p.auth.Role(s.userID, src.Team)
	return member
}

// ---- auth handlers ----

type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type meResponse struct {
	Authenticated bool        `json:"authenticated"`
	NeedsSetup    bool        `json:"needsSetup"`
	User          *userView   `json:"user,omitempty"`
	Teams         []auth.Team `json:"teams,omitempty"`
	Team          string      `json:"team,omitempty"`
}

func (p *Panel) handleMe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s, ok := p.session(r)
	if !ok {
		_ = json.NewEncoder(w).Encode(meResponse{NeedsSetup: !p.auth.HasUsers()})
		return
	}
	u, _ := p.auth.User(s.userID)
	_ = json.NewEncoder(w).Encode(meResponse{
		Authenticated: true,
		User:          &userView{ID: u.ID, Email: u.Email, Name: u.Name},
		Teams:         p.auth.TeamsForUser(s.userID),
		Team:          s.teamID,
	})
}

func (p *Panel) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if p.auth.HasUsers() {
		http.Error(w, "already set up", http.StatusConflict)
		return
	}
	var body struct{ Secret, Email, Name, Password string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	if subtle.ConstantTimeCompare([]byte(body.Secret), []byte(p.setupSecret)) != 1 {
		http.Error(w, "wrong setup secret", http.StatusUnauthorized)
		return
	}
	u, team, err := p.auth.CreateUser(body.Email, body.Name, body.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.setSession(w, u.ID, team.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct{ Email, Password string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	u, ok := p.auth.Authenticate(body.Email, body.Password)
	if !ok {
		http.Error(w, "wrong email or password", http.StatusUnauthorized)
		return
	}
	teamID := ""
	if teams := p.auth.TeamsForUser(u.ID); len(teams) > 0 {
		teamID = teams[0].ID
	}
	p.setSession(w, u.ID, teamID)
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("skiff_session"); err == nil {
		deleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "skiff_session", Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// handleAccept joins an invited user to a team (creating their account if new).
func (p *Panel) handleAccept(w http.ResponseWriter, r *http.Request) {
	var body struct{ Token, Name, Password string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	inv, ok := p.auth.Invite(body.Token)
	if !ok {
		http.Error(w, "invite not found or already used", http.StatusBadRequest)
		return
	}
	var user auth.User
	if existing, found := p.auth.UserByEmail(inv.Email); found {
		u, valid := p.auth.Authenticate(inv.Email, body.Password)
		if !valid {
			http.Error(w, "wrong password for "+existing.Email, http.StatusUnauthorized)
			return
		}
		user = u
	} else {
		u, _, err := p.auth.CreateUser(inv.Email, body.Name, body.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user = u
	}
	team, err := p.auth.AcceptInvite(body.Token, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.setSession(w, user.ID, team.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) handleTeamSwitch(w http.ResponseWriter, r *http.Request) {
	var body struct{ Team string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	s, _ := p.session(r)
	if _, ok := p.auth.Role(s.userID, body.Team); !ok {
		http.Error(w, "not a member of that team", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie("skiff_session"); err == nil {
		setSessionTeam(c.Value, body.Team)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) handleTeamCreate(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.TrimSpace(body.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	s, _ := p.session(r)
	team, err := p.auth.CreateTeam(strings.TrimSpace(body.Name), s.userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(team)
}

func (p *Panel) handleMembers(w http.ResponseWriter, r *http.Request) {
	s, _ := p.session(r)
	if _, ok := p.auth.Role(s.userID, s.teamID); !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.auth.Members(s.teamID))
}

func (p *Panel) handleInvite(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email, Role string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	s, _ := p.session(r)
	if role, ok := p.auth.Role(s.userID, s.teamID); !ok || role != auth.RoleOwner {
		http.Error(w, "only owners can invite", http.StatusForbidden)
		return
	}
	inv, err := p.auth.CreateInvite(body.Email, s.teamID, body.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"link": baseURL(r) + "/invite/" + inv.Token,
	})
}

// ---- projects ----

type appView struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	URL    string `json:"url"`
	Repo   string `json:"repo,omitempty"`
	Branch string `json:"branch,omitempty"`
	Auto   bool   `json:"auto"`
}

// handleSystem reports the control plane itself: whether it self-deploys, the
// repo it tracks, and its own deploy history (recorded under app "panel").
func (p *Panel) handleSystem(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"selfDeploy": p.selfRepo != "",
		"repo":       p.selfRepo,
		"branch":     p.selfBranch,
		"deploys":    appDeploys("panel"),
	})
}

func (p *Panel) handleApps(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	apps, err := registry.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]appView, 0)
	for _, a := range apps {
		src, ok := getSource(a.Name)
		if !ok || src.Team != team {
			continue // only this team's projects
		}
		out = append(out, appView{
			Name:   a.Name,
			State:  p.eng.State(a.Container),
			URL:    "https://" + a.Name + "." + p.domain,
			Repo:   src.Repo,
			Branch: src.Branch,
			Auto:   src.Auto,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleDeploy deploys from a pasted git URL (with an optional token), scoped to
// the caller's team, recording history + a persisted log streamed over SSE.
func (p *Panel) handleDeploy(w http.ResponseWriter, r *http.Request) {
	git := strings.TrimSpace(r.URL.Query().Get("git"))
	name := sanitizeName(r.URL.Query().Get("name"))
	port := strings.TrimSpace(r.URL.Query().Get("port"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	rootDir := strings.TrimSpace(r.URL.Query().Get("rootdir"))
	if git == "" || name == "" {
		http.Error(w, "git url and name are required", http.StatusBadRequest)
		return
	}
	if existing, ok := getSource(name); ok && existing.Team != p.teamID(r) {
		http.Error(w, "an app with that name exists in another team", http.StatusConflict)
		return
	}
	if port == "" {
		port = "3000"
	}
	src := Source{App: name, Port: port, CloneURL: git, RootDir: rootDir, Team: p.teamID(r)}
	_ = putSource(src)

	auth := ""
	if token != "" {
		auth = injectToken(git, token)
	}
	id := newDeployID()
	go p.runDeploy(src, auth, "", "manual", id)
	p.tailLog(w, r, name, id)
}

func (p *Panel) handleLogs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("app")
	if !p.canAccess(r, name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	apps, err := registry.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	app, ok := apps[name]
	if !ok {
		http.Error(w, "unknown app", http.StatusNotFound)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	p.eng.StreamLogsSSE(r.Context(), app.Container, w, fl.Flush)
}

func (p *Panel) handleDown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("app")
	if !p.canAccess(r, name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	apps, _ := registry.Load()
	if app, ok := apps[name]; ok {
		_ = p.eng.Remove(app.Container)
	}
	_, _ = registry.Delete(name)
	deleteSource(name)
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) handleEnv(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	if !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(getEnv(app))
	case http.MethodPut:
		var body struct {
			Vars []EnvVar `json:"vars"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := setEnv(app, body.Vars); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---- helpers ----

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func injectToken(gitURL, token string) string {
	const p = "https://"
	if strings.HasPrefix(gitURL, p) {
		return p + token + "@" + gitURL[len(p):]
	}
	return gitURL
}

func cloneError(b []byte) string {
	var last, fatal string
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		last = ln
		if strings.Contains(ln, "fatal:") || strings.Contains(ln, "error:") {
			fatal = ln
		}
	}
	msg := fatal
	if msg == "" {
		msg = last
	}
	if strings.Contains(msg, "could not read Username") ||
		strings.Contains(msg, "Authentication failed") ||
		strings.Contains(msg, "terminal prompts disabled") {
		return "repository is private or not found — connect GitHub, add a token, or make it public"
	}
	return msg
}

func randToken() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
