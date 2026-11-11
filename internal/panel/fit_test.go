package panel

import (
	"strings"
	"testing"
)

func TestFitCheck(t *testing.T) {
	plenty := resourceUsage{Total: 100 << 30, Used: 10 << 30}
	mem := resourceUsage{Total: 4 << 30, Used: 3 << 30}
	if w, r := fitCheck(Source{Memory: "512m"}, plenty, mem, 0); w != "" || r != "" {
		t.Errorf("512 MB fits in 1 GB free: %q %q", w, r)
	}
	w, r := fitCheck(Source{Memory: "1g", Replicas: 2}, plenty, mem, 0)
	if r != "" || !strings.Contains(w, "up to 2.0 GB for 2 copies") || !strings.Contains(w, "about 1.0 GB free") {
		t.Errorf("2 GB in 1 GB free: %q %q", w, r)
	}
	if w, _ := fitCheck(Source{Memory: "1g", Replicas: 2}, plenty, mem, 1<<30); w != "" {
		t.Errorf("the old version's memory comes back: %q", w)
	}
	if w, r := fitCheck(Source{}, plenty, mem, 0); w != "" || r != "" {
		t.Errorf("no limit, nothing to compare: %q %q", w, r)
	}
	full := resourceUsage{Total: 100 << 30, Used: 100<<30 - 500<<20}
	if _, r := fitCheck(Source{}, full, mem, 0); !strings.Contains(r, "500 MB of disk free") {
		t.Errorf("a full disk refuses: %q", r)
	}
	w, r = fitCheck(Source{Image: "ghcr.io/acme/api:fix"}, full, mem, 0)
	if r != "" || !strings.Contains(w, "pulling a new image may fail") {
		t.Errorf("an image deploy on a full disk should warn, not refuse: %q %q", w, r)
	}
	for in, want := range map[string]int64{"512m": 512 << 20, "1g": 1 << 30, "1.5g": 3 << 29, "": 0, "nope": 0, "2048k": 2 << 20, "1gb": 1 << 30, "1GiB": 1 << 30, "512MB": 512 << 20, "1024": 1024, "1024b": 1024} {
		if got := limitBytes(in); got != want {
			t.Errorf("limitBytes(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestPreviewsGiveWayFirst(t *testing.T) {
	openTestDB(t)
	if oomScoreFor(Source{App: "web"}) >= oomScoreFor(Source{App: "web-pr-1", Parent: "web"}) {
		t.Error("a preview should give way before production")
	}
	if !strings.Contains(projectToml(Source{App: "web-pr-1", Parent: "web"}, nil, "net", nil), "oom_score_adj = 600") {
		t.Error("a preview's deploy should carry its score")
	}
	if strings.Contains(projectToml(Source{App: "web"}, nil, "net", nil), "oom_score_adj") {
		t.Error("production uses the default")
	}
}

func TestAPreviewIsCheckedAgainstItsProjectsLimit(t *testing.T) {
	openTestDB(t)
	putSource(Source{App: "web", Team: "t1", Port: "3000", Memory: "3g"})
	mem := resourceUsage{Total: 4 << 30, Used: 3 << 30}
	plenty := resourceUsage{Total: 100 << 30, Used: 10 << 30}
	if w, _ := fitCheck(Source{App: "web-pr-1", Parent: "web"}, plenty, mem, 0); !strings.Contains(w, "up to 3.0 GB") {
		t.Errorf("a preview runs with its project's limit: %q", w)
	}
}

func TestFitCheckKeepsBothWarnings(t *testing.T) {
	full := resourceUsage{Total: 100 << 30, Used: 100<<30 - 500<<20}
	mem := resourceUsage{Total: 4 << 30, Used: 3 << 30}
	w, r := fitCheck(Source{Image: "ghcr.io/acme/api:fix", Memory: "2g"}, full, mem, 0)
	if r != "" || !strings.Contains(w, "pulling a new image may fail") || !strings.Contains(w, "this app may use up to") {
		t.Errorf("both warnings should show: %q %q", w, r)
	}
}
