package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nuelScript/skiff/internal/registry"
)

// The /api/v1 surface is the stable, token-authenticated API — the same actions
// the dashboard drives (list apps, deploy, poll a deploy, read and write env),
// shaped as plain JSON so they fit into CI. Every request is scoped to the
// token's team; there is no cross-team access.

type ctxKey int

const (
	ctxTeam ctxKey = iota
	ctxActor
)

func bearerToken(r *http.Request) string {
	if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// apiAuth authenticates a bearer token and pins the request to its team.
func (p *Panel) apiAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		team, name, ok := resolveToken(bearerToken(r))
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="skiff"`)
			apiErr(w, http.StatusUnauthorized, "invalid or missing API token")
			return
		}
		ctx := context.WithValue(r.Context(), ctxTeam, team)
		ctx = context.WithValue(ctx, ctxActor, "token:"+name)
		h(w, r.WithContext(ctx))
	}
}

func apiTeam(r *http.Request) string  { t, _ := r.Context().Value(ctxTeam).(string); return t }
func apiActor(r *http.Request) string { a, _ := r.Context().Value(ctxActor).(string); return a }

func apiJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, code int, msg string) {
	apiJSON(w, code, map[string]string{"error": msg})
}

// apiSource resolves an app the token's team owns (nil for unknown / other team).
func apiSource(r *http.Request, name string) (Source, bool) {
	src, ok := getSource(name)
	if !ok || src.Team != apiTeam(r) {
		return Source{}, false
	}
	return src, true
}

type apiApp struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	URL       string `json:"url"`
	Repo      string `json:"repo,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Replicas  int    `json:"replicas"`
	Running   int    `json:"running"`
	Autoscale bool   `json:"autoscale"`
	Commit    string `json:"commit,omitempty"`
	Updated   int64  `json:"updated,omitempty"`
}

func (p *Panel) apiAppView(a registry.App, src Source) apiApp {
	v := apiApp{
		Name: a.Name, State: p.eng.State(a.Container),
		URL: "https://" + a.Name + "." + p.domain,
		Repo: src.Repo, Branch: src.Branch,
		Replicas: src.Replicas, Running: len(a.Replicas), Autoscale: src.Autoscale,
	}
	if v.Running == 0 {
		v.Running = 1
	}
	if ds := appDeploys(a.Name); len(ds) > 0 {
		v.Commit, v.Updated = ds[0].Commit, ds[0].Started
	}
	return v
}

func (p *Panel) apiListApps(w http.ResponseWriter, r *http.Request) {
	team := apiTeam(r)
	apps, err := registry.List()
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []apiApp{}
	for _, a := range apps {
		src, ok := getSource(a.Name)
		if !ok || src.Team != team || src.Parent != "" {
			continue // this team's production apps only
		}
		out = append(out, p.apiAppView(a, src))
	}
	apiJSON(w, http.StatusOK, out)
}

func (p *Panel) apiGetApp(w http.ResponseWriter, r *http.Request) {
	name := sanitizeName(r.PathValue("name"))
	src, ok := apiSource(r, name)
	if !ok {
		apiErr(w, http.StatusNotFound, "app not found")
		return
	}
	apps, _ := registry.Load()
	apiJSON(w, http.StatusOK, p.apiAppView(apps[name], src))
}

func (p *Panel) apiDeploy(w http.ResponseWriter, r *http.Request) {
	name := sanitizeName(r.PathValue("name"))
	src, ok := apiSource(r, name)
	if !ok {
		apiErr(w, http.StatusNotFound, "app not found")
		return
	}
	id := newDeployID()
	recordAudit(apiTeam(r), apiActor(r), "deploy", name, "via api")
	go p.runDeploy(src, "", "", "", "api", id)
	apiJSON(w, http.StatusAccepted, map[string]string{"id": id, "app": name, "status": "building"})
}

func (p *Panel) apiDeployStatus(w http.ResponseWriter, r *http.Request) {
	d, ok := getDeployByID(sanitizeID(r.PathValue("id")))
	if !ok {
		apiErr(w, http.StatusNotFound, "deploy not found")
		return
	}
	if src, ok := getSource(d.App); !ok || src.Team != apiTeam(r) {
		apiErr(w, http.StatusNotFound, "deploy not found")
		return
	}
	apiJSON(w, http.StatusOK, d)
}

func (p *Panel) apiEnv(w http.ResponseWriter, r *http.Request) {
	name := sanitizeName(r.PathValue("name"))
	if _, ok := apiSource(r, name); !ok {
		apiErr(w, http.StatusNotFound, "app not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		apiJSON(w, http.StatusOK, getEnv(name))
	case http.MethodPut:
		var body struct {
			Vars []EnvVar `json:"vars"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			apiErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := setEnv(name, body.Vars); err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		recordAudit(apiTeam(r), apiActor(r), "env.update", name, "via api")
		w.WriteHeader(http.StatusNoContent)
	}
}
