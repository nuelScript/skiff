// Package panel is Skiff's hosted control panel: a login-gated web app with
// accounts + teams, where teams own projects deployed from git and managed on
// the box. Served behind the router at dash.<domain>.
package panel

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	reconcileStuckDeploys() // clear builds orphaned by a previous process
	home, _ := os.UserHomeDir()
	branch := os.Getenv("SKIFF_SELF_BRANCH")
	if branch == "" {
		branch = "main"
	}
	p := &Panel{
		setupSecret: setupSecret,
		domain:      domain,
		eng:         eng,
		buildsDir:   filepath.Join(home, ".skiff", "builds"),
		auth:        auth.NewStore(database),
		selfRepo:    os.Getenv("SKIFF_SELF_REPO"),
		selfBranch:  branch,
	}
	resStats = newResStore(filepath.Join(home, ".skiff", "resources.json"))
	go p.reapOrphanContainers() // clean up containers from deleted apps / failed swaps
	go func() { _ = eng.EnsureNetwork(dbNetwork) }()
	go p.reconcileNetworks()     // attach existing databases to their team's private net
	go p.prewarmDatabaseImages() // fetch DB images ahead of first provision
	go p.prewarmStorageImages()  // fetch MinIO images ahead of first bucket
	go p.backupLoop()            // daily database snapshots
	go p.jobLoop()               // scheduled jobs (cron)
	go p.resourceLoop()          // sample per-app CPU/memory
	go p.autoscaleLoop()         // add/retire replicas off those metrics
	go p.alertLoop()             // health + error-rate alerts
	return p, nil
}

// reapOrphanContainers removes skiff-managed containers that aren't the current
// version of any registered app (deleted apps, or orphans from a failed swap).
// It skips very recent ones so a deploy in progress during startup isn't hit.
func (p *Panel) reapOrphanContainers() {
	apps, err := registry.Load()
	if err != nil {
		return
	}
	current := make(map[string]bool, len(apps))
	for _, a := range apps {
		current[a.Container] = true
		for _, rp := range a.Replicas {
			current[rp.Container] = true
		}
	}
	cutoff := time.Now().Add(-10 * time.Minute)
	for _, c := range p.eng.SkiffContainers() {
		if current[c.Name] {
			continue
		}
		if c.Created.IsZero() || c.Created.Before(cutoff) {
			_ = p.eng.Stop(c.Name)
			_ = p.eng.Remove(c.Name)
		}
	}
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
	mux.HandleFunc("/api/account", p.protected(p.handleAccount))
	mux.HandleFunc("/api/account/password", p.protected(p.handlePassword))
	mux.HandleFunc("/api/account/delete", p.protected(p.handleAccountDelete))
	mux.HandleFunc("/api/teams", p.protected(p.handleTeamCreate))
	mux.HandleFunc("/api/teams/rename", p.protected(p.handleTeamRename))
	mux.HandleFunc("/api/teams/leave", p.protected(p.handleTeamLeave))
	mux.HandleFunc("/api/teams/delete", p.protected(p.handleTeamDelete))
	mux.HandleFunc("/api/teams/members", p.protected(p.handleMembers))
	mux.HandleFunc("/api/teams/invite", p.protected(p.handleInvite))
	// projects
	mux.HandleFunc("/api/system", p.protected(p.handleSystem))
	mux.HandleFunc("/api/server", p.protected(p.handleServer))
	mux.HandleFunc("/api/apps", p.protected(p.handleApps))
	mux.HandleFunc("/api/project", p.protected(p.handleProject))
	mux.HandleFunc("/api/redeploy", p.protected(p.handleRedeploy))
	mux.HandleFunc("/api/rollback", p.protected(p.handleRollback))
	mux.HandleFunc("/api/cancel", p.protected(p.handleCancel))
	mux.HandleFunc("/api/exec", p.protected(p.handleExec))
	mux.HandleFunc("/api/databases", p.protected(p.handleDatabases))
	mux.HandleFunc("/api/databases/attach", p.protected(p.handleDatabaseAttach))
	mux.HandleFunc("/api/databases/public", p.protected(p.handleDatabasePublic))
	mux.HandleFunc("/api/storage", p.protected(p.handleStorage))
	mux.HandleFunc("/api/storage/attach", p.protected(p.handleStorageAttach))
	mux.HandleFunc("/api/backups", p.protected(p.handleBackups))
	mux.HandleFunc("/api/backups/restore", p.protected(p.handleBackupRestore))
	mux.HandleFunc("/api/backups/download", p.protected(p.handleBackupDownload))
	mux.HandleFunc("/api/jobs", p.protected(p.handleJobs))
	mux.HandleFunc("/api/jobs/run", p.protected(p.handleJobRun))
	mux.HandleFunc("/api/workers", p.protected(p.handleWorkers))
	mux.HandleFunc("/api/db/exec", p.protected(p.handleDBShell))
	mux.HandleFunc("/api/domains", p.protected(p.handleDomains))
	mux.HandleFunc("/api/preview", p.protected(p.handleCreatePreview))
	mux.HandleFunc("/api/shared-env", p.protected(p.handleSharedEnv))
	mux.HandleFunc("/api/analytics", p.protected(p.handleAnalytics))
	mux.HandleFunc("/api/resources", p.protected(p.handleResources))
	mux.HandleFunc("/api/alerts", p.protected(p.handleAlerts))
	mux.HandleFunc("/api/alerts/test", p.protected(p.handleAlertTest))
	mux.HandleFunc("/api/audit", p.protected(p.handleAudit))
	mux.HandleFunc("/api/tokens", p.protected(p.handleTokens))

	// Public API v1 — token-authenticated, stable JSON for CI.
	mux.HandleFunc("GET /api/v1/apps", p.apiAuth(p.apiListApps))
	mux.HandleFunc("GET /api/v1/apps/{name}", p.apiAuth(p.apiGetApp))
	mux.HandleFunc("POST /api/v1/apps/{name}/deploy", p.apiAuth(p.apiDeploy))
	mux.HandleFunc("GET /api/v1/apps/{name}/env", p.apiAuth(p.apiEnv))
	mux.HandleFunc("PUT /api/v1/apps/{name}/env", p.apiAuth(p.apiEnv))
	mux.HandleFunc("GET /api/v1/deploys/{id}", p.apiAuth(p.apiDeployStatus))
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
	// SameSite=Strict is the CSRF defense: the browser won't attach the session
	// to any cross-site request, so a crafted link can't trigger a deploy. The SPA
	// is same-origin so its own API calls still carry it. Secure in production
	// (served over HTTPS behind the router); off for local http.
	http.SetCookie(w, &http.Cookie{
		Name: "skiff_session", Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   p.domain != "" && p.domain != "localhost",
	})
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
