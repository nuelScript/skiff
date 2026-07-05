package panel

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/nuelScript/skiff/internal/registry"
)

// Preview environments deploy a project's branch as its own throwaway app at
// <name>.<domain>, where name = "<app>-<branch>" slugified. They ride the same
// subdomain routing + automatic certs as any app — no edge changes needed.

// previewName derives a DNS-safe subdomain label for an app's branch preview.
func previewName(app, branch string) string {
	slug := slugify(app + "-" + branch)
	if len(slug) > 50 {
		slug = strings.Trim(slug[:50], "-")
	}
	return slug
}

// slugify lowercases and reduces a string to [a-z0-9-] with no leading, trailing
// or repeated dashes — a valid DNS label.
func slugify(s string) string {
	var b strings.Builder
	dash := true // avoid a leading dash
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func previewSources(parent string) []Source {
	rows, err := sqlDB.Query(`SELECT `+sourceCols+` FROM sources WHERE parent=? ORDER BY app`, parent)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		if s, ok := scanSource(rows); ok {
			out = append(out, s)
		}
	}
	return out
}

type previewView struct {
	Name    string `json:"name"`
	Branch  string `json:"branch"`
	URL     string `json:"url"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Updated int64  `json:"updated"`
}

// buildPreviews assembles the live view of a project's preview environments.
func (p *Panel) buildPreviews(parent string) []previewView {
	srcs := previewSources(parent)
	out := make([]previewView, 0, len(srcs))
	apps, _ := registry.Load()
	for _, s := range srcs {
		pv := previewView{
			Name:   s.App,
			Branch: s.Branch,
			URL:    "https://" + s.App + "." + p.domain,
			State:  "missing",
		}
		if a, ok := apps[s.App]; ok {
			pv.State = p.eng.State(a.Container)
		}
		if ds := appDeploys(s.App); len(ds) > 0 {
			pv.Status = ds[0].Status
			pv.Updated = ds[0].Started
		}
		out = append(out, pv)
	}
	return out
}

// handleCreatePreview deploys a branch of a project as a preview environment,
// inheriting the project's repo, build config and env, and streams the build.
func (p *Panel) handleCreatePreview(w http.ResponseWriter, r *http.Request) {
	parent := sanitizeName(r.URL.Query().Get("app"))
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if !p.canAccess(r, parent) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	src, ok := getSource(parent)
	if !ok || src.Parent != "" {
		http.Error(w, "unknown project", http.StatusNotFound)
		return
	}
	if branch == "" {
		http.Error(w, "a branch is required", http.StatusBadRequest)
		return
	}
	name, id, ok := p.createPreview(src, branch, "", fmt.Sprintf("preview of %s", branch))
	if !ok {
		http.Error(w, "couldn't derive a preview name from that branch", http.StatusBadRequest)
		return
	}
	p.tailLog(w, r, name, id)
}

// createPreview provisions (or updates) a preview environment for a project's
// branch — inheriting the project's source, build config and env — and starts
// its deploy. Shared by the manual endpoint and webhook auto-creation.
func (p *Panel) createPreview(parent Source, branch, commit, message string) (name, id string, ok bool) {
	name = previewName(parent.App, branch)
	if name == "" || name == parent.App {
		return "", "", false
	}
	// auto so a push to that branch redeploys the preview through the webhook path.
	pv := Source{
		App: name, Team: parent.Team, Repo: parent.Repo, Branch: branch,
		RootDir: parent.RootDir, Port: parent.Port, CloneURL: parent.CloneURL,
		Auto: true, Parent: parent.App,
	}
	_ = putSource(pv)
	_ = setEnv(name, getEnv(parent.App)) // start from the project's environment
	id = newDeployID()
	go p.runDeploy(pv, "", commit, message, "preview", id)
	return name, id, true
}

// removeAppImages deletes every retained image for an app (used on teardown so
// throwaway previews don't leave build layers behind).
func (p *Panel) removeAppImages(app string) {
	for _, t := range p.eng.AppImageTags(app) {
		_ = p.eng.RemoveImage(fmt.Sprintf("skiff-%s:%s", app, t))
	}
	_ = p.eng.RemoveImage("skiff-" + app + ":latest")
}
