package docker

import (
	"strconv"
	"strings"
)

// ContainerResource is a live CPU/memory sample for one app container, tagged
// with the skiff.app it belongs to.
type ContainerResource struct {
	App       string
	Container string
	CPUPerc   float64 // percent of one core; can exceed 100 on multi-core
	MemBytes  int64
	MemLimit  int64
	Restarts  int
}

// AppResourceStats samples every running app container in a single `docker stats`
// read, tags each by its skiff.app label, and folds in the container restart
// count. Returns nil (no error) when nothing is running.
func (e *Engine) AppResourceStats() ([]ContainerResource, error) {
	out, err := e.command("ps", "--filter", "label=skiff.app", "--format", `{{.Names}} {{.Label "skiff.app"}}`).Output()
	if err != nil {
		return nil, err
	}
	appOf := map[string]string{}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		appOf[f[0]] = f[1]
		names = append(names, f[0])
	}
	if len(names) == 0 {
		return nil, nil
	}

	stats, err := e.command(append([]string{"stats", "--no-stream", "--format", "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"}, names...)...).Output()
	if err != nil {
		return nil, err
	}
	restarts := e.restartCounts(names)

	var res []ContainerResource
	for _, line := range strings.Split(strings.TrimSpace(string(stats)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		name := strings.TrimSpace(f[0])
		app := appOf[name]
		if app == "" {
			continue
		}
		var used, limit int64
		if u, l, ok := strings.Cut(f[2], "/"); ok {
			used, limit = parseSize(u), parseSize(l)
		}
		res = append(res, ContainerResource{
			App:       app,
			Container: name,
			CPUPerc:   parsePercent(f[1]),
			MemBytes:  used,
			MemLimit:  limit,
			Restarts:  restarts[name],
		})
	}
	return res, nil
}

// restartCounts reads each container's cumulative restart count in one inspect.
func (e *Engine) restartCounts(names []string) map[string]int {
	counts := map[string]int{}
	out, err := e.command(append([]string{"inspect", "--format", "{{.Name}}\t{{.RestartCount}}"}, names...)...).Output()
	if err != nil {
		return counts
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, n, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		c, _ := strconv.Atoi(strings.TrimSpace(n))
		counts[strings.TrimPrefix(strings.TrimSpace(name), "/")] = c
	}
	return counts
}

// parsePercent turns docker's "12.34%" into 12.34.
func parsePercent(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// parseSize turns docker's human byte sizes ("12.3MiB", "7.63GiB", "512kB")
// into bytes. Binary and decimal unit suffixes are both accepted.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToLower(strings.TrimSpace(s[i:]))
	mult := 1.0
	switch {
	case strings.HasPrefix(unit, "ki"), unit == "kb", unit == "k":
		mult = 1 << 10
	case strings.HasPrefix(unit, "mi"), unit == "mb", unit == "m":
		mult = 1 << 20
	case strings.HasPrefix(unit, "gi"), unit == "gb", unit == "g":
		mult = 1 << 30
	case strings.HasPrefix(unit, "ti"), unit == "tb", unit == "t":
		mult = 1 << 40
	}
	return int64(num * mult)
}
