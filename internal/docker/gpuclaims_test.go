package docker

import (
	"strings"
	"testing"
	"time"
)

// A GPU another app has claimed but not started on yet counts as taken; the app's own claims don't stop it.
func TestPickClaimedSkipsOthersClaims(t *testing.T) {
	all := []GPU{{Index: 0, UUID: "g0"}, {Index: 1, UUID: "g1"}}
	claims := []gpuClaim{{App: "llm", GPU: "g0"}}
	got, err := pickClaimed("ollama", 1, all, nil, claims)
	if err != nil || len(got) != 1 || got[0] != "g1" {
		t.Fatalf("picked %v %v", got, err)
	}
	claims = append(claims, gpuClaim{App: "ollama", GPU: "g1"})
	if _, err := pickClaimed("vision", 1, all, nil, claims); err == nil || !strings.Contains(err.Error(), "llm (starting)") {
		t.Fatalf("both cards claimed: %v", err)
	}
	if got, err := pickClaimed("llm", 1, all, nil, claims); err != nil || got[0] != "g0" {
		t.Fatalf("an app's own claim: %v %v", got, err)
	}
}

func TestGPUClaimsLapseAndRelease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now().Unix()
	if err := updateGPUClaims(func([]gpuClaim) []gpuClaim {
		return []gpuClaim{{App: "a", GPU: "g0", Token: "t1", Until: now + 60}, {App: "b", GPU: "g1", Token: "t2", Until: now - 1}}
	}); err != nil {
		t.Fatal(err)
	}
	var seen []gpuClaim
	_ = updateGPUClaims(func(c []gpuClaim) []gpuClaim { seen = c; return c })
	if len(seen) != 1 || seen[0].App != "a" {
		t.Fatalf("after one lapsed: %+v", seen)
	}
}
