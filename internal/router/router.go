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
	"strings"

	"golang.org/x/crypto/acme/autocert"

	"github.com/nuelScript/skiff/internal/docker"
)

type Router struct {
	Domains []string
	Engine  *docker.Engine
	Panel   string // host:port of the control panel; dash.<domain> proxies here
	SiteApp string // app that serves the apex + www.<domain>
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
		if rt.Panel != "" {
			proxyTo(w, r, rt.Panel)
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
