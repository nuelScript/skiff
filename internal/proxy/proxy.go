// Package proxy routes *.localhost requests to locally deployed apps.
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/registry"
)

const DefaultAddr = ":8080"

// Proxy serves *.localhost requests, round-robining across each app's replicas.
// It implements http.Handler.
type Proxy struct {
	mu sync.Mutex
	rr map[string]uint64 // per-app round-robin cursor
}

func New() *Proxy { return &Proxy{rr: map[string]uint64{}} }

// pickPort round-robins across an app's replicas (falling back to the single
// representative port when none are recorded).
func (p *Proxy) pickPort(app registry.App) int {
	if len(app.Replicas) == 0 {
		return app.HostPort
	}
	p.mu.Lock()
	i := p.rr[app.Name] % uint64(len(app.Replicas))
	p.rr[app.Name]++
	p.mu.Unlock()
	return app.Replicas[i].HostPort
}

func URL(name string) string {
	return fmt.Sprintf("http://%s.localhost%s", name, portSuffix(DefaultAddr))
}

func portSuffix(addr string) string {
	p := strings.TrimPrefix(addr, ":")
	if p == "" || p == "80" {
		return ""
	}
	return ":" + p
}

func Serve(addr string) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           New(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := appName(r.Host)
	if name == "" {
		http.Error(w, "skiff: no app in host "+r.Host, http.StatusNotFound)
		return
	}

	apps, err := registry.Load()
	if err != nil {
		http.Error(w, "skiff: "+err.Error(), http.StatusInternalServerError)
		return
	}
	app, ok := apps[name]
	if !ok {
		http.Error(w, "skiff: no app named "+name, http.StatusNotFound)
		return
	}

	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", p.pickPort(app)))
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()       // X-Forwarded-For / -Host / -Proto
			pr.Out.Host = pr.In.Host // let the app see its real public hostname
		},
	}
	rp.ServeHTTP(w, r)
}

func appName(host string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.TrimSuffix(host, ".localhost")
	if i := strings.IndexByte(host, '.'); i >= 0 {
		host = host[:i]
	}
	return host
}
