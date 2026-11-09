package panel

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/registry"
)

// promWindow is how far back request counts reach: they're kept per minute, so a short window says what's happening now.
const promWindow = 5 * time.Minute

// promMetric is one metric family, written in Prometheus's text format.
type promMetric struct {
	name, help string
	rows       []string
}

func (m *promMetric) add(labels string, v float64) {
	m.rows = append(m.rows, fmt.Sprintf("%s{%s} %g", m.name, labels, v))
}

func (m *promMetric) write(b *strings.Builder) {
	if len(m.rows) == 0 {
		return
	}
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n", m.name, m.help, m.name)
	for _, r := range m.rows {
		b.WriteString(r + "\n")
	}
}

// promLabel quotes a label value as Prometheus expects.
func promLabel(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
}

// apiMetrics serves the team's apps in Prometheus's format, for any monitoring tool that scrapes it with an API token.
func (p *Panel) apiMetrics(w http.ResponseWriter, r *http.Request) {
	team := apiTeam(r)
	now := time.Now()
	apps, _ := registry.Load()
	traffic := readMetricsFile()
	sizes := p.volumeSizes()

	up := &promMetric{name: "skiff_app_up", help: "1 when the app has a running container."}
	crashing := &promMetric{name: "skiff_app_crashing", help: "1 when the app keeps exiting and being restarted."}
	cpu := &promMetric{name: "skiff_app_cpu_percent", help: "CPU use, in percent of one core."}
	mem := &promMetric{name: "skiff_app_memory_bytes", help: "Memory in use."}
	memLimit := &promMetric{name: "skiff_app_memory_limit_bytes", help: "The memory limit, when the app has one."}
	restarts := &promMetric{name: "skiff_app_restarts_recent", help: "Restarts in the newest sample."}
	replicas := &promMetric{name: "skiff_app_replicas", help: "How many copies are meant to run."}
	requests := &promMetric{name: "skiff_app_requests_5m", help: "Requests through the router in the last five minutes."}
	errors5 := &promMetric{name: "skiff_app_server_errors_5m", help: "Responses with a 5xx status in the last five minutes."}
	latency := &promMetric{name: "skiff_app_latency_ms_avg_5m", help: "Average response time in the last five minutes."}
	bytesIn := &promMetric{name: "skiff_app_bytes_in_5m", help: "Bytes received through the router in the last five minutes."}
	bytesOut := &promMetric{name: "skiff_app_bytes_out_5m", help: "Bytes sent through the router in the last five minutes."}
	volume := &promMetric{name: "skiff_volume_bytes", help: "Size of an app's volume."}

	sources := p.teamProjects(team)
	slices.SortFunc(sources, func(a, b Source) int { return strings.Compare(a.App, b.App) })
	for _, src := range sources {
		l := fmt.Sprintf(`app="%s"`, promLabel(src.App))
		running := 0.0
		if a, ok := apps[src.App]; ok && p.eng.State(a.Container) == "running" {
			running = 1
		}
		up.add(l, running)
		looping := 0.0
		if appCrashLoop(src.App) != nil {
			looping = 1
		}
		crashing.add(l, looping)
		replicas.add(l, float64(max(src.Replicas, 1)))
		if b, ok := resStats.newest(src.App); ok {
			cpu.add(l, b.CPUSum/float64(b.N))
			mem.add(l, float64(b.MemSum/int64(b.N)))
			if b.MemLimit > 0 {
				memLimit.add(l, float64(b.MemLimit))
			}
			restarts.add(l, float64(b.Restarts))
		}
		var req, s5, bi, bo int
		var lat int64
		for _, b := range traffic.Apps[src.App] {
			if now.Unix()-b.T <= int64(promWindow/time.Second) {
				req, s5, lat, bi, bo = req+b.Req, s5+b.S5, lat+b.Lat, bi+int(b.Bi), bo+int(b.Bo)
			}
		}
		requests.add(l, float64(req))
		errors5.add(l, float64(s5))
		if req > 0 {
			latency.add(l, float64(lat)/float64(req))
		}
		bytesIn.add(l, float64(bi))
		bytesOut.add(l, float64(bo))
		for _, v := range listVolumes(src.App) {
			volume.add(fmt.Sprintf(`app="%s",volume="%s"`, promLabel(src.App), promLabel(v.Name)), float64(sizes[config.VolumeName(src.App, v.Name)]))
		}
	}

	var b strings.Builder
	for _, m := range []*promMetric{up, crashing, cpu, mem, memLimit, restarts, replicas, requests, errors5, latency, bytesIn, bytesOut, volume} {
		m.write(&b)
	}
	// The server's own figures belong to the team that set it up, as on the Server page.
	if team != "" && team == foundingTeam() {
		u := usageOf()
		server := []*promMetric{
			{name: "skiff_server_memory_bytes", help: "Memory in use on the server."},
			{name: "skiff_server_memory_total_bytes", help: "The server's memory."},
			{name: "skiff_server_disk_bytes", help: "Disk in use on the server."},
			{name: "skiff_server_disk_total_bytes", help: "The server's disk."},
		}
		server[0].rows = []string{fmt.Sprintf("skiff_server_memory_bytes %d", u.Mem.Used)}
		server[1].rows = []string{fmt.Sprintf("skiff_server_memory_total_bytes %d", u.Mem.Total)}
		server[2].rows = []string{fmt.Sprintf("skiff_server_disk_bytes %d", u.Disk.Used)}
		server[3].rows = []string{fmt.Sprintf("skiff_server_disk_total_bytes %d", u.Disk.Total)}
		for _, m := range server {
			m.write(&b)
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
