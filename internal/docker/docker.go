// Package docker deploys apps to a Docker engine, local or remote over SSH.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Engine struct {
	host string
}

func Local() *Engine { return &Engine{} }

func Remote(sshTarget string) *Engine { return &Engine{host: "ssh://" + sshTarget} }

func For(host string) *Engine {
	if host == "" {
		return Local()
	}
	return Remote(host)
}

func (e *Engine) IsRemote() bool { return e.host != "" }

func SSHHostname(target string) string {
	if i := strings.LastIndexByte(target, '@'); i >= 0 {
		return target[i+1:]
	}
	return target
}

func (e *Engine) command(args ...string) *exec.Cmd {
	return e.contextCommand(context.Background(), args...)
}

func (e *Engine) contextCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", args...)
	if e.host != "" {
		cmd.Env = append(os.Environ(), "DOCKER_HOST="+e.host)
	}
	return cmd
}

func (e *Engine) Available() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found in PATH")
	}
	if out, err := e.command("info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		detail := lastLine(out)
		if e.IsRemote() {
			msg := "can't reach Docker on " + strings.TrimPrefix(e.host, "ssh://") +
				" over SSH — check the server is up, SSH works, and Docker is installed"
			if detail != "" {
				msg += " (" + detail + ")"
			}
			return fmt.Errorf("%s", msg)
		}
		if detail == "" {
			detail = "is Docker running?"
		}
		return fmt.Errorf("local Docker unavailable: %s", detail)
	}
	return nil
}

func (e *Engine) BuildFromDockerfile(ctx context.Context, tag, dockerfile, contextDir string, out io.Writer) error {
	defer ensureDockerignore(contextDir)()

	cmd := e.contextCommand(ctx, "build", "-t", tag, "-f", "-", contextDir)
	cmd.Stdin = strings.NewReader(dockerfile)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

type RunSpec struct {
	Name          string
	App           string // app name, for the skiff.app route label
	Image         string
	ContainerPort int
	Memory        string // optional, e.g. "512m"
	CPU           string // optional, e.g. "0.5"
	Env           map[string]string
	Public        bool   // publish on all interfaces instead of 127.0.0.1
	Network       string // optional docker network to join (for reaching managed resources by name)
}

// Route is a discovered app-to-hostport mapping (from container labels).
type Route struct {
	App      string
	HostPort int
}

func (e *Engine) Run(s RunSpec) (int, error) {
	_ = e.command("rm", "-f", s.Name).Run() // best-effort: drop the old one

	bind := "127.0.0.1"
	if s.Public {
		bind = "0.0.0.0"
	}
	args := []string{"run", "-d",
		"--name", s.Name,
		"--restart", "unless-stopped",
		"--label", "skiff=1",
		"-e", fmt.Sprintf("PORT=%d", s.ContainerPort),
		"-p", fmt.Sprintf("%s::%d", bind, s.ContainerPort),
	}
	if s.App != "" {
		args = append(args, "--label", "skiff.app="+s.App, "--label", fmt.Sprintf("skiff.port=%d", s.ContainerPort))
	}
	if s.Network != "" {
		args = append(args, "--network", s.Network)
	}
	if s.Memory != "" {
		args = append(args, "--memory", s.Memory)
	}
	if s.CPU != "" {
		args = append(args, "--cpus", s.CPU)
	}
	for _, k := range sortedKeys(s.Env) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, s.Env[k]))
	}
	args = append(args, s.Image)

	out, err := e.command(args...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("docker run failed: %s", firstLine(out))
	}
	return e.HostPort(s.Name, s.ContainerPort)
}

// EnsureNetwork creates a docker network if it doesn't already exist.
func (e *Engine) EnsureNetwork(name string) error {
	if e.command("network", "inspect", name).Run() == nil {
		return nil
	}
	if out, err := e.command("network", "create", name).CombinedOutput(); err != nil {
		return fmt.Errorf("network create failed: %s", firstLine(out))
	}
	return nil
}

// ConnectNetwork attaches a running container to an additional network. It's a
// no-op error (already connected) when the container is already a member.
func (e *Engine) ConnectNetwork(network, container string) error {
	out, err := e.command("network", "connect", network, container).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out))
	}
	return nil
}

// DBRunSpec runs a managed resource (a database) — network-internal, backed by a
// named volume, and deliberately unlabeled with skiff=1 so the app reaper leaves
// it alone.
type DBRunSpec struct {
	Name    string
	Image   string
	Network string
	Volume  string // named volume for persistence
	MountAt string // where the volume mounts inside the container
	Env     map[string]string
	Cmd     []string          // optional command/args after the image
	Labels  map[string]string // ownership + kind labels
	Port    int               // container port (published on the host when Publish is set)
	Publish bool              // expose Port on 0.0.0.0 for external access
}

// RunDatabase (re)creates a managed database container. It returns the published
// host port when Publish is set (0 otherwise). Recreating with the same name +
// volume preserves the data.
func (e *Engine) RunDatabase(s DBRunSpec) (int, error) {
	_ = e.command("rm", "-f", s.Name).Run()
	args := []string{"run", "-d", "--name", s.Name, "--restart", "unless-stopped"}
	for _, k := range sortedKeys(s.Labels) {
		args = append(args, "--label", k+"="+s.Labels[k])
	}
	if s.Network != "" {
		args = append(args, "--network", s.Network)
	}
	if s.Volume != "" && s.MountAt != "" {
		args = append(args, "-v", s.Volume+":"+s.MountAt)
	}
	if s.Publish && s.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("0.0.0.0::%d", s.Port))
	}
	for _, k := range sortedKeys(s.Env) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, s.Env[k]))
	}
	args = append(args, s.Image)
	args = append(args, s.Cmd...)
	if out, err := e.command(args...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("docker run failed: %s", firstLine(out))
	}
	if s.Publish && s.Port > 0 {
		return e.HostPort(s.Name, s.Port)
	}
	return 0, nil
}

// RemoveVolume deletes a named volume (used when tearing down a database).
func (e *Engine) RemoveVolume(name string) error {
	return e.command("volume", "rm", "-f", name).Run()
}

// PullImage fetches an image ahead of time so a later run doesn't block on it.
func (e *Engine) PullImage(image string) error {
	if out, err := e.command("pull", image).CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull failed: %s", firstLine(out))
	}
	return nil
}

// RunOnce runs a throwaway container from an image with the given env on a
// network, executing a shell command. Returns combined output and the exit
// error — used for release commands (migrations) and scheduled jobs.
func (e *Engine) RunOnce(ctx context.Context, image string, env map[string]string, network, cmd string) (string, error) {
	args := []string{"run", "--rm"}
	if network != "" {
		args = append(args, "--network", network)
	}
	for _, k := range sortedKeys(env) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, env[k]))
	}
	args = append(args, image, "sh", "-c", cmd)
	out, err := e.contextCommand(ctx, args...).CombinedOutput()
	return string(out), err
}

// Exec runs a command inside a container, wiring stdin/stdout to the given
// streams — used to dump a database to a file and pipe a dump back in.
func (e *Engine) Exec(ctx context.Context, container string, cmd []string, stdin io.Reader, stdout io.Writer) error {
	args := append([]string{"exec", "-i", container}, cmd...)
	c := e.contextCommand(ctx, args...)
	c.Stdin = stdin
	c.Stdout = stdout
	var errb bytes.Buffer
	c.Stderr = &errb
	if err := c.Run(); err != nil {
		if msg := firstLine(errb.Bytes()); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

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
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// AppContainers lists every container (running or not) for an app, so all stale
// versions can be retired — not just the one the registry last recorded.
func (e *Engine) AppContainers(app string) []string {
	out, err := e.command("ps", "-a", "--filter", "label=skiff.app="+app, "--format", "{{.Names}}").Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names
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
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
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
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
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
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
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

func (e *Engine) Stop(container string) error {
	out, err := e.command("stop", container).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out))
	}
	return nil
}

func (e *Engine) Remove(name string) error {
	out, err := e.command("rm", "-f", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out))
	}
	return nil
}

// Tag adds an additional name to an existing image, so a build can be retained
// as a rollback point (e.g. skiff-app:latest -> skiff-app:<deployid>).
func (e *Engine) Tag(src, dst string) error {
	out, err := e.command("tag", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out))
	}
	return nil
}

// AppImageTags lists the retained tags of an app's images (skiff-<app>:*),
// newest first, excluding :latest and dangling <none>. Docker lists images
// created-descending by default, which is the order we rely on for pruning.
func (e *Engine) AppImageTags(app string) []string {
	out, err := e.command("images", "skiff-"+app, "--format", "{{.Tag}}").Output()
	if err != nil {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		t = strings.TrimSpace(t)
		if t == "" || t == "latest" || t == "<none>" {
			continue
		}
		tags = append(tags, t)
	}
	return tags
}

// ImageExists reports whether a tagged image is present locally.
func (e *Engine) ImageExists(tag string) bool {
	return e.command("image", "inspect", tag).Run() == nil
}

// RemoveImage deletes a tagged image (best-effort; ignores "in use").
func (e *Engine) RemoveImage(tag string) error {
	return e.command("rmi", tag).Run()
}

func (e *Engine) Logs(container string, follow bool, tail string, out io.Writer) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "--follow")
	}
	if tail != "" {
		args = append(args, "--tail", tail)
	}
	args = append(args, container)
	cmd := e.command(args...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

func (e *Engine) StreamLogs(ctx context.Context, container string, out io.Writer) error {
	cmd := e.contextCommand(ctx, "logs", "--tail", "100", "--follow", container)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// StreamLogsSSE follows a container's logs and writes them as SSE data frames,
// flushing after each line.
func (e *Engine) StreamLogsSSE(ctx context.Context, container string, w io.Writer, flush func()) {
	pr, pw := io.Pipe()
	go func() { _ = e.StreamLogs(ctx, container, pw); pw.Close() }()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		fmt.Fprintf(w, "data: %s\n\n", sc.Text())
		flush()
	}
}

// ensureDockerignore writes a sensible default .dockerignore when the app has
// none, so host junk (node_modules, .git) doesn't bloat or corrupt the image.
// The returned cleanup removes only a file we created.
func ensureDockerignore(dir string) func() {
	p := filepath.Join(dir, ".dockerignore")
	if _, err := os.Stat(p); err == nil {
		return func() {} // the app ships its own — leave it alone
	}
	if err := os.WriteFile(p, []byte("node_modules\n.git\n.skiff\n.env\n*.log\n"), 0o644); err != nil {
		return func() {}
	}
	return func() { os.Remove(p) }
}

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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func firstLine(b []byte) string {
	if i := strings.IndexByte(string(b), '\n'); i >= 0 {
		return string(b[:i])
	}
	return strings.TrimSpace(string(b))
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
