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

// clientIP is the caller's address. X-Forwarded-For is only trusted when the
// direct peer is loopback — i.e. the request came through Skiff's edge router on
// this box, which replaces any client-supplied XFF with the real client IP. A
// non-loopback peer reaching the panel directly could forge XFF to dodge the
// login lockout, so its real remote address is used instead.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	return host
}
