package router

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// Request metrics: the router buckets traffic per app, per minute, keeping a
// rolling window in memory and snapshotting it to a file the panel reads for the
// Analytics page. The window is seeded from that file on startup so a router
// restart doesn't lose recent history.

const (
	metricsWindowSecs int64 = 24 * 60 * 60 // keep ~24h of buckets
	metricsBucketSecs int64 = 60           // one bucket per minute
)

// Bucket is one minute of traffic for an app.
type Bucket struct {
	T      int64 `json:"t"` // minute, unix seconds (floored)
	Req    int   `json:"req"`
	S2     int   `json:"s2"`
	S3     int   `json:"s3"`
	S4     int   `json:"s4"`
	S5     int   `json:"s5"`
	LatMs  int64 `json:"lat"` // summed latency in ms (÷ Req for the average)
	BytesI int64 `json:"bi"`  // request bytes in
	BytesO int64 `json:"bo"`  // response bytes out
}

type metricsSnapshot struct {
	Updated int64               `json:"updated"`
	Apps    map[string][]Bucket `json:"apps"`
}

type Metrics struct {
	mu   sync.Mutex
	apps map[string]map[int64]*Bucket
	file string
}

func NewMetrics(file string) *Metrics {
	m := &Metrics{apps: map[string]map[int64]*Bucket{}, file: file}
	m.load()
	return m
}

// Record adds one request to the current minute's bucket for an app.
func (m *Metrics) Record(app string, status int, bytesIn, bytesOut int64, dur time.Duration) {
	if app == "" {
		return
	}
	t := (time.Now().Unix() / metricsBucketSecs) * metricsBucketSecs
	m.mu.Lock()
	defer m.mu.Unlock()
	buckets := m.apps[app]
	if buckets == nil {
		buckets = map[int64]*Bucket{}
		m.apps[app] = buckets
	}
	b := buckets[t]
	if b == nil {
		b = &Bucket{T: t}
		buckets[t] = b
	}
	b.Req++
	switch {
	case status >= 500:
		b.S5++
	case status >= 400:
		b.S4++
	case status >= 300:
		b.S3++
	default:
		b.S2++
	}
	b.LatMs += dur.Milliseconds()
	if bytesIn > 0 {
		b.BytesI += bytesIn
	}
	b.BytesO += bytesOut
}

// Run prunes the window and snapshots to disk on a fixed cadence, for the life
// of the process.
func (m *Metrics) Run() {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for range tick.C {
		m.mu.Lock()
		m.prune(time.Now().Unix())
		snap := m.buildSnapshot()
		m.mu.Unlock()
		m.write(snap)
	}
}

func (m *Metrics) prune(now int64) {
	cutoff := now - metricsWindowSecs
	for app, buckets := range m.apps {
		for t := range buckets {
			if t < cutoff {
				delete(buckets, t)
			}
		}
		if len(buckets) == 0 {
			delete(m.apps, app)
		}
	}
}

func (m *Metrics) buildSnapshot() metricsSnapshot {
	snap := metricsSnapshot{Updated: time.Now().Unix(), Apps: map[string][]Bucket{}}
	for app, buckets := range m.apps {
		arr := make([]Bucket, 0, len(buckets))
		for _, b := range buckets {
			arr = append(arr, *b)
		}
		sort.Slice(arr, func(i, j int) bool { return arr[i].T < arr[j].T })
		snap.Apps[app] = arr
	}
	return snap
}

func (m *Metrics) write(snap metricsSnapshot) {
	if m.file == "" {
		return
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return
	}
	tmp := m.file + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, m.file)
	}
}

func (m *Metrics) load() {
	if m.file == "" {
		return
	}
	b, err := os.ReadFile(m.file)
	if err != nil {
		return
	}
	var snap metricsSnapshot
	if json.Unmarshal(b, &snap) != nil {
		return
	}
	for app, arr := range snap.Apps {
		buckets := map[int64]*Bucket{}
		for i := range arr {
			bb := arr[i]
			buckets[bb.T] = &bb
		}
		m.apps[app] = buckets
	}
}

// statusRecorder captures the response status and byte count while forwarding
// everything else (including Flush, via Unwrap) so proxied SSE/streaming works.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer's Flush/Hijack.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
