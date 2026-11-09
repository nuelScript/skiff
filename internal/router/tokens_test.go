package router

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestAccessTokensGuardAnApp(t *testing.T) {
	var sawAuth []string
	app := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sawAuth = append(sawAuth, r.Header.Get("Authorization"))
	}))
	defer app.Close()
	u, _ := url.Parse(app.URL)
	port, _ := strconv.Atoi(u.Port())
	sum := sha256.Sum256([]byte("skat_good"))
	rules := filepath.Join(t.TempDir(), "edge.json")
	_ = os.WriteFile(rules, []byte(`{"mcp":{"tokens":["`+hex.EncodeToString(sum[:])+`"]}}`), 0o644)
	rt := &Router{Domains: []string{"example.com"}, EdgeFile: rules}
	rt.routeTable = map[string][]int{"mcp": {port}, "web": {port}}
	rt.routeAt = time.Now().Add(time.Hour)
	send := func(host, method, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/mcp", nil)
		req.Host, req.RemoteAddr = host, "1.1.1.1:5000"
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		rt.ServeHTTP(w, req)
		return w
	}
	for _, bad := range []string{"", "Bearer skat_wrong", "Basic c2thdF9nb29k", "Bearer "} {
		if w := send("mcp.example.com", "POST", bad); w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%q: %d", bad, w.Code)
		}
	}
	if len(sawAuth) != 0 {
		t.Fatalf("refused requests reached the app: %v", sawAuth)
	}
	if w := send("mcp.example.com", "POST", "Bearer skat_good"); w.Code != 200 {
		t.Fatalf("with the token: %d", w.Code)
	}
	if sawAuth[0] != "" {
		t.Errorf("the token reached the app: %q", sawAuth[0])
	}
	if w := send("mcp.example.com", "OPTIONS", ""); w.Code != 200 {
		t.Errorf("a browser's preflight: %d", w.Code)
	}
	if w := send("web.example.com", "GET", ""); w.Code != 200 {
		t.Errorf("an app without tokens: %d", w.Code)
	}
}
