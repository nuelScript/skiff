package panel

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Analytics reads the per-app request metrics the router snapshots and
// aggregates them for the caller's team over a recent window.

type metricBucket struct {
	T   int64 `json:"t"`
	Req int   `json:"req"`
	S2  int   `json:"s2"`
	S3  int   `json:"s3"`
	S4  int   `json:"s4"`
	S5  int   `json:"s5"`
	Lat int64 `json:"lat"`
}

type metricsFile struct {
	Updated int64                     `json:"updated"`
	Apps    map[string][]metricBucket `json:"apps"`
}

func readMetricsFile() metricsFile {
	m := metricsFile{Apps: map[string][]metricBucket{}}
	b, err := os.ReadFile(filepath.Join(skiffDir(), "metrics.json"))
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	if m.Apps == nil {
		m.Apps = map[string][]metricBucket{}
	}
	return m
}

type analyticsPoint struct {
	T   int64 `json:"t"`
	Req int   `json:"req"`
}

type analyticsApp struct {
	Name     string `json:"name"`
	Req      int    `json:"req"`
	AvgLatMs int    `json:"avgLatMs"`
}

type analyticsResponse struct {
	WindowMins int `json:"windowMins"`
	Total      int `json:"total"`
	Status     struct {
		S2 int `json:"s2"`
		S3 int `json:"s3"`
		S4 int `json:"s4"`
		S5 int `json:"s5"`
	} `json:"status"`
	Series  []analyticsPoint `json:"series"`
	Apps    []analyticsApp   `json:"apps"`
	Updated int64            `json:"updated"`
}

func (p *Panel) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	data := readMetricsFile()

	const windowMins = 60
	now := time.Now().Unix()
	startMin := ((now - windowMins*60) / 60) * 60
	nowMin := (now / 60) * 60

	perMin := map[int64]int{}
	type acc struct {
		req int
		lat int64
	}
	perApp := map[string]*acc{}
	var resp analyticsResponse

	for app, buckets := range data.Apps {
		src, ok := getSource(app)
		if !ok || src.Team != team {
			continue // only this team's apps
		}
		for _, b := range buckets {
			if b.T < startMin {
				continue
			}
			perMin[b.T] += b.Req
			resp.Status.S2 += b.S2
			resp.Status.S3 += b.S3
			resp.Status.S4 += b.S4
			resp.Status.S5 += b.S5
			a := perApp[app]
			if a == nil {
				a = &acc{}
				perApp[app] = a
			}
			a.req += b.Req
			a.lat += b.Lat
		}
	}

	// A continuous per-minute series (zero-filled) makes for a clean chart.
	resp.Series = make([]analyticsPoint, 0, windowMins+1)
	for t := startMin; t <= nowMin; t += 60 {
		req := perMin[t]
		resp.Total += req
		resp.Series = append(resp.Series, analyticsPoint{T: t, Req: req})
	}

	resp.Apps = make([]analyticsApp, 0, len(perApp))
	for name, a := range perApp {
		avg := 0
		if a.req > 0 {
			avg = int(a.lat / int64(a.req))
		}
		resp.Apps = append(resp.Apps, analyticsApp{Name: name, Req: a.req, AvgLatMs: avg})
	}
	sort.Slice(resp.Apps, func(i, j int) bool { return resp.Apps[i].Req > resp.Apps[j].Req })
	if len(resp.Apps) > 8 {
		resp.Apps = resp.Apps[:8]
	}

	resp.WindowMins = windowMins
	resp.Updated = data.Updated
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
