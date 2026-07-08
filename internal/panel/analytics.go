package panel

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type metricBucket struct {
	T   int64 `json:"t"`
	Req int   `json:"req"`
	S2  int   `json:"s2"`
	S3  int   `json:"s3"`
	S4  int   `json:"s4"`
	S5  int   `json:"s5"`
	Lat int64 `json:"lat"`
	Bi  int64 `json:"bi"`
	Bo  int64 `json:"bo"`
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

type analyticsSeries struct {
	T   int64 `json:"t"`
	Req int   `json:"req"`
	S2  int   `json:"s2"`
	S3  int   `json:"s3"`
	S4  int   `json:"s4"`
	S5  int   `json:"s5"`
	Bi  int64 `json:"bi"`
	Bo  int64 `json:"bo"`
	Lat int   `json:"lat"`
}

type analyticsApp struct {
	Name     string `json:"name"`
	Req      int    `json:"req"`
	AvgLatMs int    `json:"avgLatMs"`
}

type analyticsResponse struct {
	RangeMins  int `json:"rangeMins"`
	BucketSecs int `json:"bucketSecs"`
	Total      int `json:"total"`
	Status     struct {
		S2 int `json:"s2"`
		S3 int `json:"s3"`
		S4 int `json:"s4"`
		S5 int `json:"s5"`
	} `json:"status"`
	BytesIn    int64             `json:"bytesIn"`
	BytesOut   int64             `json:"bytesOut"`
	AvgLatMs   int               `json:"avgLatMs"`
	Series     []analyticsSeries `json:"series"`
	Apps       []analyticsApp    `json:"apps"`
	AppOptions []string          `json:"appOptions"`
	Updated    int64             `json:"updated"`
}

// clampRange keeps the requested window within [15m, 24h], defaulting to 1h.
func clampRange(v string) int {
	n, _ := strconv.Atoi(v)
	if n < 15 {
		return 60
	}
	if n > 1440 {
		return 1440
	}
	return n
}

// displayBucket picks a bucket size that yields ~72 points across the window.
func displayBucket(rangeMins int) int64 {
	target := int64(rangeMins) * 60 / 72
	if target < 60 {
		return 60
	}
	return ((target + 59) / 60) * 60
}

func (p *Panel) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	only := sanitizeName(r.URL.Query().Get("app")) // "" = all team apps
	rangeMins := clampRange(r.URL.Query().Get("range"))
	bucketSecs := displayBucket(rangeMins)

	data := readMetricsFile()
	now := time.Now().Unix()
	startT := ((now - int64(rangeMins)*60) / bucketSecs) * bucketSecs
	nowT := (now / bucketSecs) * bucketSecs

	type agg struct {
		s2, s3, s4, s5, req int
		bi, bo, lat         int64
	}
	buckets := map[int64]*agg{}
	type appAcc struct {
		req int
		lat int64
	}
	apps := map[string]*appAcc{}
	var totalLat int64
	var resp analyticsResponse
	appOptions := []string{}

	for app, bs := range data.Apps {
		src, ok := getSource(app)
		if !ok || src.Team != team {
			continue
		}
		appOptions = append(appOptions, app)
		if only != "" && app != only {
			continue
		}
		for _, b := range bs {
			if b.T < startT {
				continue
			}
			bt := (b.T / bucketSecs) * bucketSecs
			a := buckets[bt]
			if a == nil {
				a = &agg{}
				buckets[bt] = a
			}
			a.s2 += b.S2
			a.s3 += b.S3
			a.s4 += b.S4
			a.s5 += b.S5
			a.req += b.Req
			a.bi += b.Bi
			a.bo += b.Bo
			a.lat += b.Lat

			resp.Status.S2 += b.S2
			resp.Status.S3 += b.S3
			resp.Status.S4 += b.S4
			resp.Status.S5 += b.S5
			resp.Total += b.Req
			resp.BytesIn += b.Bi
			resp.BytesOut += b.Bo
			totalLat += b.Lat

			ap := apps[app]
			if ap == nil {
				ap = &appAcc{}
				apps[app] = ap
			}
			ap.req += b.Req
			ap.lat += b.Lat
		}
	}

	resp.Series = make([]analyticsSeries, 0, rangeMins)
	for t := startT; t <= nowT; t += bucketSecs {
		s := analyticsSeries{T: t}
		if a := buckets[t]; a != nil {
			s.S2, s.S3, s.S4, s.S5, s.Req = a.s2, a.s3, a.s4, a.s5, a.req
			s.Bi, s.Bo = a.bi, a.bo
			if a.req > 0 {
				s.Lat = int(a.lat / int64(a.req))
			}
		}
		resp.Series = append(resp.Series, s)
	}
	if resp.Total > 0 {
		resp.AvgLatMs = int(totalLat / int64(resp.Total))
	}

	resp.Apps = make([]analyticsApp, 0, len(apps))
	for name, a := range apps {
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
	sort.Strings(appOptions)

	resp.AppOptions = appOptions
	resp.RangeMins = rangeMins
	resp.BucketSecs = int(bucketSecs)
	resp.Updated = data.Updated
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
