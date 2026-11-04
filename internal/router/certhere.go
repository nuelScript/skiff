package router

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	hereTTL      = 10 * time.Minute
	hereFailTTL  = time.Minute
	hereMaxNames = 10_000
)

// lookupIPs is a variable so tests can stand in for DNS.
var lookupIPs = func(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

var localIPs = func() []string {
	addrs, _ := net.InterfaceAddrs()
	var out []string
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			out = append(out, n.IP.String())
		}
	}
	return out
}

type hereEntry struct {
	ok    bool
	until time.Time
}

type hereState struct {
	mu    sync.Mutex
	names map[string]hereEntry
	own   map[string]bool
	ownAt time.Time
}

// hadCert reports whether a certificate for host is already cached, so a name that has worked before keeps renewing even if it now sits behind a proxy.
func (rt *Router) hadCert(host string) bool {
	if rt.CertDir == "" {
		return false
	}
	for _, name := range []string{host, host + "+rsa"} {
		if _, err := os.Stat(filepath.Join(rt.CertDir, name)); err == nil {
			return true
		}
	}
	return false
}

// resolvesHere reports whether host currently points at this server. Answers are cached briefly, and a failed lookup counts as no.
func (rt *Router) resolvesHere(host string) bool {
	now := time.Now()
	s := &rt.here
	s.mu.Lock()
	if e, ok := s.names[host]; ok && now.Before(e.until) {
		s.mu.Unlock()
		return e.ok
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ok := false
	if addrs, err := lookupIPs(ctx, host); err == nil {
		own := rt.ownIPs(ctx, now)
		for _, a := range addrs {
			if own[a] {
				ok = true
				break
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.names == nil || len(s.names) >= hereMaxNames {
		s.names = map[string]hereEntry{}
	}
	ttl := hereFailTTL
	if ok {
		ttl = hereTTL
	}
	s.names[host] = hereEntry{ok: ok, until: now.Add(ttl)}
	return ok
}

// ownIPs is this machine's addresses plus whatever the served base domain resolves to, which covers hosts whose public address is NATed.
func (rt *Router) ownIPs(ctx context.Context, now time.Time) map[string]bool {
	s := &rt.here
	s.mu.Lock()
	if s.own != nil && now.Sub(s.ownAt) < hereTTL {
		own := s.own
		s.mu.Unlock()
		return own
	}
	s.mu.Unlock()
	own := map[string]bool{}
	for _, ip := range localIPs() {
		own[ip] = true
	}
	if len(rt.Domains) > 0 {
		if addrs, err := lookupIPs(ctx, rt.Domains[0]); err == nil {
			for _, a := range addrs {
				own[a] = true
			}
		}
	}
	s.mu.Lock()
	s.own, s.ownAt = own, now
	s.mu.Unlock()
	return own
}
