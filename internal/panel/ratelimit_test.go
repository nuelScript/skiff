package panel

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginThrottle(t *testing.T) {
	tr := newLoginThrottle()
	now := time.Unix(1_700_000_000, 0)
	const ip = "203.0.113.7"

	// Up to the limit stays allowed; the limit-th failure trips the block.
	for i := 0; i < tr.max; i++ {
		if tr.blocked(ip, now) {
			t.Fatalf("blocked early at attempt %d", i)
		}
		tr.fail(ip, now)
	}
	if !tr.blocked(ip, now) {
		t.Fatal("client not blocked after hitting the failure limit")
	}

	// A different client is unaffected.
	if tr.blocked("198.51.100.9", now) {
		t.Fatal("an unrelated client was blocked")
	}

	// The window expires.
	if tr.blocked(ip, now.Add(tr.window+time.Second)) {
		t.Fatal("still blocked after the window elapsed")
	}

	// A successful login clears prior failures immediately.
	for i := 0; i < tr.max; i++ {
		tr.fail(ip, now)
	}
	tr.ok(ip)
	if tr.blocked(ip, now) {
		t.Fatal("still blocked after a successful login cleared the window")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := clientIP(r); got != "203.0.113.7" {
		t.Fatalf("forwarded client IP = %q, want 203.0.113.7", got)
	}

	r2 := httptest.NewRequest("POST", "/api/auth/login", nil)
	r2.RemoteAddr = "192.0.2.5:41000"
	if got := clientIP(r2); got != "192.0.2.5" {
		t.Fatalf("remote-addr client IP = %q, want 192.0.2.5", got)
	}
}
