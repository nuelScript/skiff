package panel

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginThrottle blunts online password guessing: too many failed logins from one client trip a 429 until the window cools off (in-memory; a restart clears it).
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

func (t *loginThrottle) ok(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.windows, key)
}

// clientIP trusts X-Forwarded-For only from a loopback peer (Skiff's edge router, which sets the real IP); a direct non-loopback peer could forge XFF to dodge the login lockout.
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
