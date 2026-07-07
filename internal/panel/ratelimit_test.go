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
	// XFF from the loopback edge router is trusted (it sets the real client IP).
	viaRouter := httptest.NewRequest("POST", "/api/auth/login", nil)
	viaRouter.RemoteAddr = "127.0.0.1:5555"
	viaRouter.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := clientIP(viaRouter); got != "203.0.113.7" {
		t.Fatalf("forwarded client IP via router = %q, want 203.0.113.7", got)
	}

	// XFF from a non-loopback peer is forgeable, so it's ignored.
	spoof := httptest.NewRequest("POST", "/api/auth/login", nil)
	spoof.RemoteAddr = "198.51.100.9:41000"
	spoof.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := clientIP(spoof); got != "198.51.100.9" {
		t.Fatalf("spoofed XFF should be ignored, got %q, want 198.51.100.9", got)
	}

	// No XFF: the connection's remote address.
	direct := httptest.NewRequest("POST", "/api/auth/login", nil)
	direct.RemoteAddr = "192.0.2.5:41000"
	if got := clientIP(direct); got != "192.0.2.5" {
		t.Fatalf("remote-addr client IP = %q, want 192.0.2.5", got)
	}
}
