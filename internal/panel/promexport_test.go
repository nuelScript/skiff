package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/docker"
)

func TestMetricsAreTheTeamsOwn(t *testing.T) {
	openTestDB(t)
	putSource(Source{App: "shop", Team: "t1", Port: "3000"})
	putSource(Source{App: "secret", Team: "t2", Port: "3000"})
	p := &Panel{eng: docker.Local()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxTeam, "t1"))
	w := httptest.NewRecorder()
	p.apiMetrics(w, req)
	body := w.Body.String()
	if !strings.Contains(body, `skiff_app_up{app="shop"} 0`) || !strings.Contains(body, "# TYPE skiff_app_up gauge") {
		t.Errorf("metrics:\n%s", body)
	}
	if strings.Contains(body, "secret") {
		t.Errorf("another team's app showed up:\n%s", body)
	}
	if strings.Contains(body, "skiff_server_") {
		t.Errorf("only the team that set the server up sees its figures:\n%s", body)
	}
	if got := promLabel("a\"b\\c\nd"); got != `a\"b\\c\nd` {
		t.Errorf("label = %s", got)
	}
}
