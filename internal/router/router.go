// Package router is Skiff's edge router: it discovers apps from Docker labels
// and reverse-proxies <app>.<domain> to them, with automatic HTTPS. It runs on
// the server.
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
	Domain string
	Engine *docker.Engine
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app := rt.appFor(r.Host)
	if app == "" {
		http.Error(w, "skiff: no app for "+r.Host, http.StatusNotFound)
		return
	}
	routes, err := rt.Engine.Routes()
	if err != nil {
		http.Error(w, "skiff: "+err.Error(), http.StatusInternalServerError)
		return
	}
	port := 0
	for _, rr := range routes {
		if rr.App == app {
			port = rr.HostPort
			break
		}
	}
	if port == 0 {
		http.Error(w, "skiff: no app named "+app, http.StatusNotFound)
		return
	}

	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	rp := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.SetXForwarded()
		pr.Out.Host = pr.In.Host
	}}
	rp.ServeHTTP(w, r)
}

// appFor pulls "blog" out of "blog.useskiff.xyz".
func (rt *Router) appFor(host string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if s := "." + rt.Domain; strings.HasSuffix(host, s) {
		return strings.TrimSuffix(host, s)
	}
	return ""
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
			if host == rt.Domain || strings.HasSuffix(host, "."+rt.Domain) {
				return nil
			}
			return fmt.Errorf("host not allowed: %s", host)
		},
	}
	go http.ListenAndServe(":80", m.HTTPHandler(nil))
	server := &http.Server{Addr: ":443", Handler: rt, TLSConfig: m.TLSConfig()}
	return server.ListenAndServeTLS("", "")
}
