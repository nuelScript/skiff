package router

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func stubDNS(t *testing.T, records map[string][]string, local ...string) {
	t.Helper()
	oldLookup, oldLocal := lookupIPs, localIPs
	lookupIPs = func(_ context.Context, host string) ([]string, error) {
		if a, ok := records[host]; ok {
			return a, nil
		}
		return nil, errors.New("no such host")
	}
	localIPs = func() []string { return local }
	t.Cleanup(func() { lookupIPs, localIPs = oldLookup, oldLocal })
}

// pointAllHere makes every name resolve to this server, for tests that aren't about DNS.
func pointAllHere(t *testing.T) {
	t.Helper()
	oldLookup, oldLocal := lookupIPs, localIPs
	lookupIPs = func(context.Context, string) ([]string, error) { return []string{"192.0.2.1"}, nil }
	localIPs = func() []string { return []string{"192.0.2.1"} }
	t.Cleanup(func() { lookupIPs, localIPs = oldLookup, oldLocal })
}

func TestCustomDomainCertNeedsToPointHere(t *testing.T) {
	dir := t.TempDir()
	domains := filepath.Join(dir, "domains.json")
	_ = os.WriteFile(domains, []byte(`{"here.example.org":"a","elsewhere.example.org":"a","proxied.example.org":"a","gone.example.org":"a"}`), 0o644)
	certs := filepath.Join(dir, "certs")
	_ = os.MkdirAll(certs, 0o755)
	_ = os.WriteFile(filepath.Join(certs, "proxied.example.org"), []byte("x"), 0o600)
	stubDNS(t, map[string][]string{
		"here.example.org":      {"198.51.100.7"},
		"elsewhere.example.org": {"203.0.113.9"},
		"proxied.example.org":   {"203.0.113.9"},
		"useskiff.xyz":          {"198.51.100.7"},
	})
	rt := &Router{Domains: []string{"useskiff.xyz"}, DomainsFile: domains, CertDir: certs}

	for host, want := range map[string]bool{
		"here.example.org":      true,  // resolves to the base domain's address
		"elsewhere.example.org": false, // never had a certificate and points away
		"proxied.example.org":   true,  // already has a certificate, keeps renewing
		"gone.example.org":      false, // lookup fails, so fail closed
	} {
		if got := rt.certAllowed(host); got != want {
			t.Errorf("certAllowed(%q) = %v, want %v", host, got, want)
		}
	}

	lookups := 0
	inner := lookupIPs
	lookupIPs = func(ctx context.Context, h string) ([]string, error) { lookups++; return inner(ctx, h) }
	rt.certAllowed("elsewhere.example.org")
	rt.certAllowed("elsewhere.example.org")
	if lookups != 0 {
		t.Fatalf("answers should be cached briefly, got %d lookups", lookups)
	}
	rt.here.mu.Lock()
	rt.here.names["elsewhere.example.org"] = hereEntry{until: time.Now().Add(-time.Second)}
	rt.here.mu.Unlock()
	rt.certAllowed("elsewhere.example.org")
	if lookups == 0 {
		t.Fatal("expired answers should be looked up again")
	}
}
