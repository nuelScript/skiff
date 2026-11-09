package panel

import (
	"net/http"
	"slices"
	"strings"
)

// What a token can do. A token with none listed is from before scopes and can do everything, as it always could.
const (
	scopeRead   = "read"   // look at apps, deploys, logs, workers, jobs and runs
	scopeDeploy = "deploy" // deploy, roll back, previews
	scopeEnv    = "env"    // read and change environment variables, which hold secrets
	scopeWork   = "work"   // change workers and jobs, and run commands
	scopeData   = "data"   // databases, buckets and volumes
)

var allScopes = []string{scopeRead, scopeDeploy, scopeEnv, scopeWork, scopeData}

// tokenGrant is what one token may do, and to which apps; no apps means all of the team's.
type tokenGrant struct {
	Scopes []string
	Apps   []string
}

func (g tokenGrant) can(scope string) bool {
	return len(g.Scopes) == 0 || slices.Contains(g.Scopes, scope)
}

func (g tokenGrant) limited() bool { return len(g.Apps) > 0 }

// reaches says whether the token may touch an app; a preview counts as its project.
func (g tokenGrant) reaches(src Source) bool {
	if !g.limited() {
		return true
	}
	return slices.Contains(g.Apps, src.App) || src.Parent != "" && slices.Contains(g.Apps, src.Parent)
}

// routeScope is the scope each API route needs, by its pattern.
func routeScope(pattern string) string {
	method, path, _ := strings.Cut(pattern, " ")
	switch {
	case strings.HasSuffix(path, "/env"):
		return scopeEnv
	case strings.HasPrefix(path, "/api/v1/databases"), strings.HasPrefix(path, "/api/v1/buckets"), strings.HasSuffix(path, "/volumes"), strings.HasSuffix(path, "/volumes/{volume}"):
		if method == http.MethodGet && !strings.HasSuffix(path, "/connection") && !strings.HasSuffix(path, "/tunnel") {
			return scopeRead
		}
		return scopeData
	case method == http.MethodGet:
		return scopeRead
	case strings.HasSuffix(path, "/deploy"), strings.HasSuffix(path, "/rollback"), strings.HasSuffix(path, "/previews"), path == "/api/v1/cli/deploys":
		return scopeDeploy
	}
	return scopeWork
}

// teamWide routes act on the whole team's resources rather than one app, so a token limited to some apps can't use them. Attaching a database or bucket counts too: it hands the app that resource's credentials.
func teamWide(pattern string) bool {
	_, path, _ := strings.Cut(pattern, " ")
	return path == "/api/v1/databases" || path == "/api/v1/buckets" ||
		strings.HasPrefix(path, "/api/v1/databases/{name}") || strings.HasPrefix(path, "/api/v1/buckets/{name}")
}

func apiGrant(r *http.Request) tokenGrant {
	g, _ := r.Context().Value(ctxGrant).(tokenGrant)
	return g
}

// scopeRefusal explains, in the API's words, why a token can't use a route.
func scopeRefusal(g tokenGrant, pattern string) string {
	if need := routeScope(pattern); !g.can(need) {
		return "this token can't " + map[string]string{
			scopeRead: "read", scopeDeploy: "deploy", scopeEnv: "use environment variables", scopeWork: "change workers or jobs, or run commands", scopeData: "change databases, buckets or volumes",
		}[need] + "; create one that can under Settings → Integrations"
	}
	if g.limited() && teamWide(pattern) {
		return "this token is limited to some projects, so it can't act on the team's databases and buckets"
	}
	return ""
}

func encodeList(v []string) string { return strings.Join(v, ",") }

func decodeList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
