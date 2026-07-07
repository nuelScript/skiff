package panel

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// logRequests writes a basic access line — method, path, status, duration —
// after each request completes, so the control plane's activity is visible in
// the logs (there was no request-level logging before).
func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &respRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		h.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// respRecorder captures the response status while delegating Flush and Hijack,
// so SSE log streams and the exec WebSocket keep working through the middleware.
type respRecorder struct {
	http.ResponseWriter
	status int
}

// The exec WebSocket needs Hijacker and the SSE streams need Flusher; assert
// both at compile time so a wrong signature can't silently break them.
var (
	_ http.Flusher  = (*respRecorder)(nil)
	_ http.Hijacker = (*respRecorder)(nil)
)

func (s *respRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *respRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *respRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := s.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijack not supported")
}

// Unwrap lets http.ResponseController reach the underlying writer's Flush/Hijack.
func (s *respRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
