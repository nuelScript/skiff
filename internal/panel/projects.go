package panel

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nuelScript/skiff/internal/registry"
)

type appView struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	URL     string `json:"url"`
	Repo    string `json:"repo,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Auto    bool   `json:"auto"`
	Commit  string `json:"commit,omitempty"`
	Message string `json:"message,omitempty"`
	Updated int64  `json:"updated,omitempty"`
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
		if src.Parent != "" {
			continue // previews live under their project, not the grid
		}
		av := appView{
			Name:   a.Name,
			State:  p.eng.State(a.Container),
			URL:    "https://" + a.Name + "." + p.domain,
			Repo:   src.Repo,
			Branch: src.Branch,
			Auto:   src.Auto,
		}
		if ds := appDeploys(a.Name); len(ds) > 0 {
			av.Commit = ds[0].Commit
			av.Message = ds[0].Message
			av.Updated = ds[0].Started
		}
		out = append(out, av)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

type projectView struct {
	Name        string        `json:"name"`
	State       string        `json:"state"`
	URL         string        `json:"url"`
	Repo        string        `json:"repo"`
	Branch      string        `json:"branch"`
	RootDir     string        `json:"rootDir"`
	Port        string        `json:"port"`
	Auto        bool          `json:"auto"`
	PreviewAuto bool          `json:"previewAuto"`
	Replicas    int           `json:"replicas"`
	Running     int           `json:"running"` // replicas currently up (autoscaling moves this)
	Release     string        `json:"release"`
	Autoscale   bool          `json:"autoscale"`
	ScaleMin    int           `json:"scaleMin"`
	ScaleMax    int           `json:"scaleMax"`
	ScaleCPU    int           `json:"scaleCpu"`
	Deploys     []Deploy      `json:"deploys"`
	Previews    []previewView `json:"previews"`
}

// handleProject serves one project's detail (GET) or updates its settings (PUT):
// source config, live state, URL, and deploy history — the project page.
func (p *Panel) handleProject(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	if !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		src, _ := getSource(app)
		state := "missing"
		running := 0
		if apps, err := registry.Load(); err == nil {
			if a, ok := apps[app]; ok {
				state = p.eng.State(a.Container)
				running = len(a.Replicas)
				if running == 0 {
					running = 1
				}
			}
		}
		deploys := appDeploys(app)
		p.markRollbackable(app, deploys)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(projectView{
			Name: app, State: state, URL: "https://" + app + "." + p.domain,
			Repo: src.Repo, Branch: src.Branch, RootDir: src.RootDir, Port: src.Port,
			Auto: src.Auto, PreviewAuto: src.PreviewAuto, Replicas: src.Replicas, Running: running,
			Release: src.Release, Autoscale: src.Autoscale, ScaleMin: src.ScaleMin, ScaleMax: src.ScaleMax, ScaleCPU: src.ScaleCPU,
			Deploys: deploys, Previews: p.buildPreviews(app),
		})
	case http.MethodPut:
		var body struct {
			Branch      string `json:"branch"`
			RootDir     string `json:"rootDir"`
			Port        string `json:"port"`
			Auto        bool   `json:"auto"`
			PreviewAuto bool   `json:"previewAuto"`
			Replicas    int    `json:"replicas"`
			Release     string `json:"release"`
			Autoscale   bool   `json:"autoscale"`
			ScaleMin    int    `json:"scaleMin"`
			ScaleMax    int    `json:"scaleMax"`
			ScaleCPU    int    `json:"scaleCpu"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		src, ok := getSource(app)
		if !ok {
			http.Error(w, "unknown project", http.StatusNotFound)
			return
		}
		src.Branch = strings.TrimSpace(body.Branch)
		src.RootDir = strings.TrimSpace(body.RootDir)
		if port := strings.TrimSpace(body.Port); port != "" {
			src.Port = port
		}
		if body.Replicas >= 1 && body.Replicas <= 10 {
			src.Replicas = body.Replicas
		}
		src.Release = strings.TrimSpace(body.Release)
		src.Auto = body.Auto
		src.PreviewAuto = body.PreviewAuto
		src.Autoscale = body.Autoscale
		if body.ScaleMin >= 1 && body.ScaleMin <= 10 {
			src.ScaleMin = body.ScaleMin
		}
		if body.ScaleMax >= 1 && body.ScaleMax <= 10 {
			src.ScaleMax = body.ScaleMax
		}
		if src.ScaleMax < src.ScaleMin {
			src.ScaleMax = src.ScaleMin
		}
		if body.ScaleCPU >= 10 && body.ScaleCPU <= 100 {
			src.ScaleCPU = body.ScaleCPU
		}
		_ = putSource(src)
		p.audit(r, "settings.update", app, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRedeploy rebuilds a project's current source (its latest commit) without
// needing a fresh push, streaming the build log over SSE.
func (p *Panel) handleRedeploy(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	if !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	src, ok := getSource(app)
	if !ok {
		http.Error(w, "unknown project", http.StatusNotFound)
		return
	}
	id := newDeployID()
	p.audit(r, "deploy", app, "redeploy")
	go p.runDeploy(src, "", "", "", "redeploy", id)
	p.tailLog(w, r, app, id)
}

// markRollbackable flags each past deploy whose build image is still retained
// and that isn't the version currently serving — i.e. an instant-rollback target.
func (p *Panel) markRollbackable(app string, deploys []Deploy) {
	retained := map[string]bool{}
	for _, t := range p.eng.AppImageTags(app) {
		retained[t] = true
	}
	current := ""
	for _, d := range deploys {
		if d.Status == "live" {
			current = d.ID // newest live deploy = what's running
			break
		}
	}
	for i := range deploys {
		deploys[i].Rollbackable = deploys[i].ID != current && retained[deploys[i].ID]
	}
}

// handleRollback re-runs a retained past build with no rebuild (instant
// rollback), recording it as a new deploy and streaming progress over SSE.
func (p *Panel) handleRollback(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	target := sanitizeID(r.URL.Query().Get("id"))
	if !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	src, ok := getSource(app)
	if !ok {
		http.Error(w, "unknown project", http.StatusNotFound)
		return
	}
	var commit, message string
	for _, d := range appDeploys(app) {
		if d.ID == target {
			commit, message = d.Commit, d.Message
			break
		}
	}
	id := newDeployID()
	detail := ""
	if commit != "" {
		detail = "to " + commit
	}
	p.audit(r, "rollback", app, detail)
	go p.runRollback(src, target, commit, message, id)
	p.tailLog(w, r, app, id)
}

// handleCancel stops a build: it cancels the live in-flight build for the app
// (which records it as "canceled"), or force-clears a deploy stuck at "building"
// with no live process behind it.
func (p *Panel) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	app := sanitizeName(r.URL.Query().Get("app"))
	id := sanitizeID(r.URL.Query().Get("id"))
	if !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !cancelInflight(app, id) && deployStatus(app, id) == "building" {
		setDeployStatus(app, id, "canceled") // orphaned build — no live process
	}
	w.WriteHeader(http.StatusNoContent)
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
	p.audit(r, "deploy", name, "new app from git")
	go p.runDeploy(src, auth, "", "", "manual", id)
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
	// Remove every container for the app — all replicas, not just the
	// representative — or the leftovers keep serving at the app's URL (the router
	// discovers backends by label) and hold their host ports.
	for _, c := range p.eng.AppContainers(name) {
		_ = p.eng.Remove(c)
	}
	for _, c := range p.eng.WorkerContainers(name) {
		_ = p.eng.Remove(c)
	}
	_, _ = sqlDB.Exec(`DELETE FROM workers WHERE app=?`, name)
	_, _ = registry.Delete(name)
	deleteSource(name)
	p.removeAppImages(name) // free the build layers; nothing left to roll back to
	p.audit(r, "project.delete", name, "")
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) handleEnv(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	// A not-yet-deployed app has no source; allow staging env for it so it can be
	// set from the deploy dialog. Once it exists, require team access.
	if _, exists := getSource(app); exists && !p.canAccess(r, app) {
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
		p.audit(r, "env.update", app, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSharedEnv manages the caller team's shared environment variables, which
// are merged into every app in the team on its next deploy.
func (p *Panel) handleSharedEnv(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	if team == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sharedEnv(team))
	case http.MethodPut:
		var body struct {
			Vars []EnvVar `json:"vars"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := setSharedEnv(team, body.Vars); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
