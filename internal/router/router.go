// Package router is Skiff's edge router: it discovers apps from Docker labels
// and reverse-proxies <app>.<domain> to them, with automatic HTTPS. It runs on
// the server. Reserved hosts: dash.<domain> → the control panel, status.<domain>
// → a live status page, and the apex + www.<domain> → a designated site app.
package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/nuelScript/skiff/internal/docker"
)

type Router struct {
	Domains []string
	Engine  *docker.Engine
	Panel   string // fallback host:port of the control panel; dash.<domain> proxies here
	// PanelPointer, when set, is a file holding the current panel host:port. The
	// panel can rewrite it during a zero-downtime self-deploy to flip dash.<domain>
	// onto the freshly-built process, so the router itself never restarts.
	PanelPointer string
	SiteApp      string // app that serves the apex + www.<domain>

	mu          sync.Mutex
	cachedPanel string
	cachedAt    time.Time
}

// panelUpstream resolves where dash.<domain> should proxy to. When a pointer
// file is configured it wins (cached briefly to avoid a read per request),
// otherwise the static Panel address is used.
func (rt *Router) panelUpstream() string {
	if rt.PanelPointer == "" {
		return rt.Panel
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.cachedPanel != "" && time.Since(rt.cachedAt) < 2*time.Second {
		return rt.cachedPanel
	}
	up := rt.Panel
	if b, err := os.ReadFile(rt.PanelPointer); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			up = v
		}
	}
	rt.cachedPanel, rt.cachedAt = up, time.Now()
	return up
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sub, ok := rt.subFor(r.Host)
	if !ok {
		http.Error(w, "skiff: no app for "+r.Host, http.StatusNotFound)
		return
	}

	switch sub {
	case "status":
		rt.serveStatus(w, r)
		return
	case "dash":
		if up := rt.panelUpstream(); up != "" {
			proxyTo(w, r, up)
			return
		}
	}

	app := sub
	if sub == "" || sub == "www" {
		app = rt.SiteApp
	}
	if app == "" {
		http.Error(w, "skiff: no app for "+r.Host, http.StatusNotFound)
		return
	}

	routes, err := rt.Engine.Routes()
	if err != nil {
		http.Error(w, "skiff: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, rr := range routes {
		if rr.App == app {
			proxyTo(w, r, fmt.Sprintf("127.0.0.1:%d", rr.HostPort))
			return
		}
	}
	http.Error(w, "skiff: no app named "+app, http.StatusNotFound)
}

func proxyTo(w http.ResponseWriter, r *http.Request, hostport string) {
	target, _ := url.Parse("http://" + hostport)
	rp := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.SetXForwarded()
		pr.Out.Host = pr.In.Host
	}}
	rp.ServeHTTP(w, r)
}

// subFor classifies a host under a served domain: "" for the apex, otherwise the
// leading label ("dash", "status", "www", "blog"). ok is false when the host is
// not under any served domain.
func (rt *Router) subFor(host string) (string, bool) {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, d := range rt.Domains {
		if host == d {
			return "", true
		}
		if s := "." + d; strings.HasSuffix(host, s) {
			return strings.TrimSuffix(host, s), true
		}
	}
	return "", false
}

// ServeHTTPOnly runs the router over plain HTTP (for local testing, no TLS).
func (rt *Router) ServeHTTPOnly(addr string) error {
	return http.ListenAndServe(addr, rt)
}

// ServeTLS runs :443 with Let's Encrypt certs and :80 for ACME challenges +
// an HTTP→HTTPS redirect.
func (rt *Router) ServeTLS(cacheDir string) error {
	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache(cacheDir),
		HostPolicy: func(_ context.Context, host string) error {
			for _, d := range rt.Domains {
				if host == d || strings.HasSuffix(host, "."+d) {
					return nil
				}
			}
			return fmt.Errorf("host not allowed: %s", host)
		},
	}
	go http.ListenAndServe(":80", m.HTTPHandler(nil))
	server := &http.Server{Addr: ":443", Handler: rt, TLSConfig: m.TLSConfig()}
	return server.ListenAndServeTLS("", "")
}
