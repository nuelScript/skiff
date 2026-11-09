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

func TestMetricsForATokenLimitedToSomeApps(t *testing.T) {
	openTestDB(t)
	_, _ = sqlDB.Exec(`INSERT INTO teams(id, name, slug, created) VALUES('t1', 'T', 't', 1)`)
	putSource(Source{App: "shop", Team: "t1", Port: "3000"})
	putSource(Source{App: "admin", Team: "t1", Port: "3000"})
	p := &Panel{eng: docker.Local()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	ctx := context.WithValue(req.Context(), ctxTeam, "t1")
	req = req.WithContext(context.WithValue(ctx, ctxGrant, tokenGrant{Apps: []string{"shop"}}))
	w := httptest.NewRecorder()
	p.apiMetrics(w, req)
	body := w.Body.String()
	if !strings.Contains(body, `app="shop"`) || strings.Contains(body, `app="admin"`) {
		t.Errorf("a token limited to shop:\n%s", body)
	}
	if strings.Contains(body, "skiff_server_") {
		t.Errorf("a limited token saw the server's figures:\n%s", body)
	}
}

func TestGPUMetrics(t *testing.T) {
	var b strings.Builder
	for _, m := range gpuMetrics([]docker.GPU{{Index: 0, UUID: "a", Name: "NVIDIA L4", MemoryUsed: 1024, MemoryTotal: 23034, Utilization: 37, Temperature: 45}}, map[string]string{"a": "llm"}) {
		m.write(&b)
	}
	for _, want := range []string{`skiff_gpu_utilization_percent{gpu="0",name="NVIDIA L4",app="llm"} 37`, `skiff_gpu_memory_used_bytes{gpu="0",name="NVIDIA L4",app="llm"} 1.073741824e+09`, `skiff_gpu_temperature_celsius{gpu="0",name="NVIDIA L4",app="llm"} 45`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %s in:\n%s", want, b.String())
		}
	}
}
