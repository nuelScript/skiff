// Package router is Skiff's edge router: it discovers apps from Docker labels
// and reverse-proxies <app>.<domain> to them, with automatic HTTPS. It runs on
// the server. Reserved hosts: dash.<domain> → the control panel, status.<domain>
// → a live status page, and the apex + www.<domain> → a designated site app.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	// DomainsFile, when set, is a JSON file of custom host→app mappings the panel
	// maintains. The router reads it live (cached briefly) so domains can be added
	// or removed without restarting the edge.
	DomainsFile string
	// MetricsFile, when set, is where the router snapshots per-app request metrics
	// for the panel's Analytics page.
	MetricsFile string

	metrics       *Metrics
	mu            sync.Mutex
	cachedPanel   string
	cachedAt      time.Time
	cachedDomains map[string]string
	cachedDomAt   time.Time
	rr            map[string]uint64 // per-app round-robin cursor across replicas

	// routesMu guards the discovered-routes cache. It's separate from mu so the
	// (slow) Docker fetch never blocks round-robin/domain lookups, and concurrent
	// cache misses coalesce behind it into a single `docker ps` instead of one per
	// in-flight request.
	routesMu      sync.Mutex
	cachedRoutes  []docker.Route
	cachedRouteAt time.Time
}

// startMetrics begins request accounting if a metrics file is configured.
func (rt *Router) startMetrics() {
	if rt.MetricsFile != "" && rt.metrics == nil {
		rt.metrics = NewMetrics(rt.MetricsFile)
		go rt.metrics.Run()
	}
}

// customDomains returns the current host→app map from DomainsFile, cached for a
// couple of seconds to avoid a read per request.
func (rt *Router) customDomains() map[string]string {
	if rt.DomainsFile == "" {
		return nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.cachedDomains != nil && time.Since(rt.cachedDomAt) < 2*time.Second {
		return rt.cachedDomains
	}
	m := map[string]string{}
	if b, err := os.ReadFile(rt.DomainsFile); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	rt.cachedDomains, rt.cachedDomAt = m, time.Now()
	return m
}

func hostOnly(host string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
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
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	label := rt.serve(rec, r)
	if rt.metrics != nil {
		rt.metrics.Record(label, rec.status, r.ContentLength, rec.bytes, time.Since(start))
	}
}

// serve routes a request and returns the app label it was accounted to (empty
// when the host matched nothing, so it isn't recorded).
func (rt *Router) serve(w http.ResponseWriter, r *http.Request) string {
	// A custom domain pointed at one of the apps wins over subdomain routing.
	if app, ok := rt.customDomains()[hostOnly(r.Host)]; ok {
		rt.proxyToApp(w, r, app)
		return app
	}

	sub, ok := rt.subFor(r.Host)
	if !ok {
		http.Error(w, "skiff: no app for "+r.Host, http.StatusNotFound)
		return ""
	}

	switch sub {
	case "status":
		rt.serveStatus(w, r)
		return "status"
	case "dash":
		if up := rt.panelUpstream(); up != "" {
			proxyTo(w, r, up)
			return "dash"
		}
	}

	app := sub
	if sub == "" || sub == "www" {
		app = rt.SiteApp
	}
	if app == "" {
		http.Error(w, "skiff: no app for "+r.Host, http.StatusNotFound)
		return ""
	}
	rt.proxyToApp(w, r, app)
	return app
}

// proxyToApp forwards the request to one of the named app's live containers,
// round-robining across its replicas.
// routes returns the current app→hostport mappings, cached for a short window so
// a burst of requests to a deployed app doesn't fork `docker ps` (plus a `docker
// port` per replica) on every single request.
func (rt *Router) routes() ([]docker.Route, error) {
	rt.routesMu.Lock()
	defer rt.routesMu.Unlock()
	if rt.cachedRoutes != nil && time.Since(rt.cachedRouteAt) < 2*time.Second {
		return rt.cachedRoutes, nil
	}
	routes, err := rt.Engine.Routes()
	if err != nil {
		return nil, err
	}
	rt.cachedRoutes, rt.cachedRouteAt = routes, time.Now()
	return routes, nil
}

func (rt *Router) proxyToApp(w http.ResponseWriter, r *http.Request, app string) {
	routes, err := rt.routes()
	if err != nil {
		http.Error(w, "skiff: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var ports []int
	for _, rr := range routes {
		if rr.App == app {
			ports = append(ports, rr.HostPort)
		}
	}
	if len(ports) == 0 {
		http.Error(w, "skiff: no app named "+app, http.StatusNotFound)
		return
	}
	rt.mu.Lock()
	if rt.rr == nil {
		rt.rr = map[string]uint64{}
	}
	port := ports[rt.rr[app]%uint64(len(ports))]
	rt.rr[app]++
	rt.mu.Unlock()
	proxyTo(w, r, fmt.Sprintf("127.0.0.1:%d", port))
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

// edgeServer builds an internet-facing HTTP server with conservative limits:
// ReadHeaderTimeout closes Slowloris slow-header attacks and IdleTimeout reaps
// idle keep-alives, while Read/Write timeouts stay unset so proxied SSE and
// WebSocket streams (deploy logs, the console) aren't cut off mid-stream.
func edgeServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// ServeHTTPOnly runs the router over plain HTTP (for local testing, no TLS).
func (rt *Router) ServeHTTPOnly(addr string) error {
	rt.startMetrics()
	return edgeServer(addr, rt).ListenAndServe()
}

// ServeTLS runs :443 with Let's Encrypt certs and :80 for ACME challenges +
// an HTTP→HTTPS redirect.
func (rt *Router) ServeTLS(cacheDir string) error {
	rt.startMetrics()
	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache(cacheDir),
		HostPolicy: func(_ context.Context, host string) error {
			for _, d := range rt.Domains {
				if host == d || strings.HasSuffix(host, "."+d) {
					return nil
				}
			}
			if _, ok := rt.customDomains()[host]; ok {
				return nil // a registered custom domain — allow its cert
			}
			return fmt.Errorf("host not allowed: %s", host)
		},
	}
	redirect := edgeServer(":80", m.HTTPHandler(nil))
	go func() {
		if err := redirect.ListenAndServe(); err != nil {
			log.Printf("router :80 listener stopped: %v", err)
		}
	}()
	server := edgeServer(":443", rt)
	server.TLSConfig = m.TLSConfig()
	return server.ListenAndServeTLS("", "")
}
