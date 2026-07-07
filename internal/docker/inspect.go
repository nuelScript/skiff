package docker

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (e *Engine) HostPort(name string, containerPort int) (int, error) {
	out, err := e.command("port", name, strconv.Itoa(containerPort)).Output()
	if err != nil {
		return 0, fmt.Errorf("reading published port: %w", err)
	}
	line := firstLine(out) // e.g. "127.0.0.1:49153"
	i := strings.LastIndexByte(line, ':')
	if i < 0 {
		return 0, fmt.Errorf("unexpected `docker port` output: %q", line)
	}
	return strconv.Atoi(strings.TrimSpace(line[i+1:]))
}

func (e *Engine) State(container string) string {
	out, err := e.command("inspect", "-f", "{{.State.Status}}", container).Output()
	if err != nil {
		return "missing"
	}
	return strings.TrimSpace(string(out))
}

func (e *Engine) Containers() ([]string, error) {
	out, err := e.command("ps", "-a", "--filter", "label=skiff=1", "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// AppContainers lists every container (running or not) for an app, so all stale
// versions can be retired — not just the one the registry last recorded.
func (e *Engine) AppContainers(app string) []string {
	out, err := e.command("ps", "-a", "--filter", "label=skiff.app="+app, "--format", "{{.Names}}").Output()
	if err != nil {
		return nil
	}
	return splitLines(out)
}

// ContainerInfo is a skiff-managed container's name and creation time.
type ContainerInfo struct {
	Name    string
	Created time.Time
}

// SkiffContainers lists all skiff-managed containers with their creation time,
// for reaping orphans (deleted apps, failed swaps) on startup.
func (e *Engine) SkiffContainers() []ContainerInfo {
	out, err := e.command("ps", "-a", "--filter", "label=skiff=1", "--format", "{{.Names}}|{{.CreatedAt}}").Output()
	if err != nil {
		return nil
	}
	var cs []ContainerInfo
	for _, line := range splitLines(out) {
		name, created, ok := strings.Cut(line, "|")
		if !ok || name == "" {
			continue
		}
		t, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", strings.TrimSpace(created))
		cs = append(cs, ContainerInfo{Name: name, Created: t})
	}
	return cs
}

// Routes discovers app-to-hostport mappings from skiff.app-labeled containers.
func (e *Engine) Routes() ([]Route, error) {
	out, err := e.command("ps", "--filter", "label=skiff.app", "--format", `{{.Label "skiff.app"}} {{.Label "skiff.port"}} {{.Names}}`).Output()
	if err != nil {
		return nil, err
	}
	var routes []Route
	for _, line := range splitLines(out) {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		cport, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		hp, err := e.HostPort(f[2], cport)
		if err != nil {
			continue
		}
		routes = append(routes, Route{App: f[0], HostPort: hp})
	}
	return routes, nil
}

// AppStates maps each skiff app to its container state (running, exited, ...).
// Running wins when an app has more than one container (e.g. mid-rollout).
func (e *Engine) AppStates() (map[string]string, error) {
	out, err := e.command("ps", "-a", "--filter", "label=skiff.app", "--format", `{{.Label "skiff.app"}}|{{.State}}`).Output()
	if err != nil {
		return nil, err
	}
	states := map[string]string{}
	for _, line := range splitLines(out) {
		app, state, ok := strings.Cut(line, "|")
		if !ok || app == "" {
			continue
		}
		if states[app] != "running" {
			states[app] = state
		}
	}
	return states, nil
}
