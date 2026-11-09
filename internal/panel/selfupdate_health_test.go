package panel

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthyWaitsForASlowStart(t *testing.T) {
	healthEvery = 10 * time.Millisecond
	t.Cleanup(func() { healthEvery = 2 * time.Second })

	var ready atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	time.AfterFunc(200*time.Millisecond, func() { ready.Store(true) })
	if !healthy(srv.URL, 5*time.Second, func() bool { return true }) {
		t.Fatal("gave up on a panel that was still starting")
	}
}

func TestHealthyStopsWhenTheProcessExits(t *testing.T) {
	healthEvery = 10 * time.Millisecond
	t.Cleanup(func() { healthEvery = 2 * time.Second })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	start := time.Now()
	if healthy(srv.URL, time.Minute, func() bool { return false }) {
		t.Fatal("reported a stopped panel as healthy")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("kept waiting after the process exited")
	}
}
