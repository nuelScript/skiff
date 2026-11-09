package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestEveryRouteNeedsTheRightScope(t *testing.T) {
	src, err := os.ReadFile("panel.go")
	if err != nil {
		t.Fatal(err)
	}
	routes := regexp.MustCompile(`HandleFunc\("([A-Z]+ /api/v1/[^"]+)", p\.apiAuth`).FindAllStringSubmatch(string(src), -1)
	if len(routes) < 40 {
		t.Fatalf("found %d API routes", len(routes))
	}
	want := map[string]string{
		"GET /api/v1/apps":                            scopeRead,
		"PUT /api/v1/apps/{name}/env":                 scopeEnv,
		"GET /api/v1/apps/{name}/env":                 scopeEnv,
		"POST /api/v1/apps/{name}/deploy":             scopeDeploy,
		"POST /api/v1/apps/{name}/rollback":           scopeDeploy,
		"POST /api/v1/cli/deploys":                    scopeDeploy,
		"POST /api/v1/apps/{name}/runs":               scopeWork,
		"PUT /api/v1/apps/{name}/workers/{worker}":    scopeWork,
		"GET /api/v1/databases":                       scopeRead,
		"GET /api/v1/databases/{name}/connection":     scopeData,
		"POST /api/v1/databases":                      scopeData,
		"PUT /api/v1/buckets/{name}/apps/{app}":       scopeData,
		"DELETE /api/v1/apps/{name}/volumes/{volume}": scopeData,
		"GET /api/v1/apps/{name}/runs/{id}/logs":      scopeRead,
	}
	seen := map[string]bool{}
	for _, m := range routes {
		pattern := m[1]
		got := routeScope(pattern)
		if w, ok := want[pattern]; ok && got != w {
			t.Errorf("%s needs %s, want %s", pattern, got, w)
		}
		if strings.HasPrefix(pattern, "GET ") && got != scopeRead && got != scopeEnv && got != scopeData {
			t.Errorf("%s reads but needs %s", pattern, got)
		}
		seen[pattern] = true
	}
	for p := range want {
		if !seen[p] {
			t.Errorf("no route %s", p)
		}
	}
}

func TestTokenLimits(t *testing.T) {
	readOnly := tokenGrant{Scopes: []string{scopeRead}, Apps: []string{"web"}}
	if msg := scopeRefusal(readOnly, "POST /api/v1/apps/{name}/deploy"); !strings.Contains(msg, "can't deploy") {
		t.Errorf("deploy with a read token: %q", msg)
	}
	if msg := scopeRefusal(readOnly, "GET /api/v1/apps/{name}"); msg != "" {
		t.Errorf("read with a read token: %q", msg)
	}
	if msg := scopeRefusal(tokenGrant{Apps: []string{"web"}}, "POST /api/v1/databases"); !strings.Contains(msg, "limited to some projects") {
		t.Errorf("a team-wide route with a limited token: %q", msg)
	}
	// Attaching hands the app the database's password, so a limited token can't attach one it was never given.
	for _, route := range []string{"PUT /api/v1/databases/{name}/apps/{app}", "DELETE /api/v1/buckets/{name}/apps/{app}"} {
		if msg := scopeRefusal(tokenGrant{Apps: []string{"web"}}, route); !strings.Contains(msg, "limited to some projects") {
			t.Errorf("%s with a limited token: %q", route, msg)
		}
	}
	if msg := scopeRefusal(tokenGrant{}, "DELETE /api/v1/buckets/{name}"); msg != "" {
		t.Errorf("an old token can do everything: %q", msg)
	}
	if !readOnly.reaches(Source{App: "web"}) || !readOnly.reaches(Source{App: "web-pr-3", Parent: "web"}) || readOnly.reaches(Source{App: "api"}) {
		t.Error("a limited token reaches its project and its previews only")
	}

	openTestDB(t)
	putSource(Source{App: "web", Team: "t1", Port: "3000"})
	putSource(Source{App: "api", Team: "t1", Port: "3000"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxTeam, "t1")
	req = req.WithContext(context.WithValue(ctx, ctxGrant, readOnly))
	if _, ok := apiSource(req, "web"); !ok {
		t.Error("its own project should be found")
	}
	if _, ok := apiSource(req, "api"); ok {
		t.Error("another project should read as not found")
	}
	if _, problem := checkGrant("t1", []string{"read", "teleport"}, nil); problem == "" {
		t.Error("an unknown scope should be refused")
	}
	if _, problem := checkGrant("t1", nil, []string{"elsewhere"}); problem == "" {
		t.Error("a project the team doesn't have should be refused")
	}
	if g, _ := checkGrant("t1", allScopes, []string{"web", "web"}); g.Scopes != nil || len(g.Apps) != 1 {
		t.Errorf("every scope is the same as none listed, and projects dedupe: %+v", g)
	}
}
