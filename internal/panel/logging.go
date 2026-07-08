package panel

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &respRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		h.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// respRecorder captures the status but delegates Flush/Hijack so SSE streams and the exec WebSocket keep working.
type respRecorder struct {
	http.ResponseWriter
	status int
}

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
