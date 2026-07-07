package panel

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginThrottle blunts online password guessing: after too many failed logins
// from one client within a window, further attempts get a 429 until it cools
// off. It's in-memory — a restart clears it, which is fine for a self-hosted
// panel behind its own edge router.
type loginThrottle struct {
	mu      sync.Mutex
	windows map[string]*failWindow
	max     int
	window  time.Duration
}

type failWindow struct {
	count int
	until time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{windows: map[string]*failWindow{}, max: 10, window: 15 * time.Minute}
}

var loginLimiter = newLoginThrottle()

// blocked reports whether key is currently locked out, dropping an expired window.
func (t *loginThrottle) blocked(key string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[key]
	if !ok {
		return false
	}
	if now.After(w.until) {
		delete(t.windows, key)
		return false
	}
	return w.count >= t.max
}

// fail records a failed attempt, opening or extending the client's window.
func (t *loginThrottle) fail(key string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[key]
	if !ok || now.After(w.until) {
		w = &failWindow{until: now.Add(t.window)}
		t.windows[key] = w
	}
	w.count++
}

// ok clears a client's failures after a successful login.
func (t *loginThrottle) ok(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.windows, key)
}

// clientIP is the caller's address. Behind Skiff's edge router the real client
// IP arrives in X-Forwarded-For (the router replaces any client-supplied value,
// so it can't be spoofed); otherwise fall back to the connection's remote addr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
