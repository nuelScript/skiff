package router

import (
	"net/http/httptest"
	"testing"

	"github.com/nuelScript/skiff/internal/docker"
)

func TestPlatformTrafficIsLabelledOutsideProjectNames(t *testing.T) {
	rt := &Router{Domains: []string{"example.com"}, metrics: NewMetrics(""), Engine: docker.Remote("nobody@127.0.0.1:1")}
	rt.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://status.example.com/x", nil))
	if len(rt.metrics.apps["status"]) != 0 || len(rt.metrics.apps["dash"]) != 0 {
		t.Fatal("platform traffic must not appear under a name a project can take")
	}
	if len(rt.metrics.apps[statusLabel]) == 0 {
		t.Fatalf("status traffic not recorded: %v", rt.metrics.apps)
	}
}
