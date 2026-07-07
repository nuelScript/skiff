package proxy

import (
	"testing"

	"github.com/nuelScript/skiff/internal/registry"
)

func TestAppName(t *testing.T) {
	cases := map[string]string{
		"myapp.localhost":      "myapp",
		"myapp.localhost:8080": "myapp",
		"api.myapp.localhost":  "api", // leading label wins
		"plainhost":            "plainhost",
		"":                     "",
	}
	for in, want := range cases {
		if got := appName(in); got != want {
			t.Errorf("appName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPortSuffix(t *testing.T) {
	cases := map[string]string{
		":8080": ":8080",
		":80":   "", // default HTTP port is elided
		":":     "",
		"":      "",
	}
	for in, want := range cases {
		if got := portSuffix(in); got != want {
			t.Errorf("portSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickPort(t *testing.T) {
	// No replicas recorded: fall back to the representative host port.
	solo := registry.App{Name: "solo", HostPort: 5000}
	for range 3 {
		if p := pickPort(solo); p != 5000 {
			t.Fatalf("single-port pick = %d, want 5000", p)
		}
	}

	// Replicas: round-robin across all of them and wrap.
	multi := registry.App{Name: "multi", Replicas: []registry.Replica{
		{HostPort: 1}, {HostPort: 2}, {HostPort: 3},
	}}
	var seq []int
	for range 4 {
		seq = append(seq, pickPort(multi))
	}
	seen := map[int]bool{seq[0]: true, seq[1]: true, seq[2]: true}
	if len(seen) != 3 {
		t.Fatalf("round-robin didn't cover all replicas: %v", seq)
	}
	if seq[3] != seq[0] {
		t.Fatalf("round-robin didn't wrap after the last replica: %v", seq)
	}
}
