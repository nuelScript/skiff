// Package proxy routes *.localhost requests to locally deployed apps.
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/nuelScript/skiff/internal/registry"
)

const DefaultAddr = ":8080"

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
	return http.ListenAndServe(addr, http.HandlerFunc(route))
}

func route(w http.ResponseWriter, r *http.Request) {
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

	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", app.HostPort))
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
