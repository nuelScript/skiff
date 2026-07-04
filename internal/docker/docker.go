// Package docker deploys apps to a Docker engine — local, or remote over SSH —
// via the docker CLI.
package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Engine talks to a Docker daemon. An empty host means the local daemon; a set
// host is a DOCKER_HOST value like "ssh://user@vps".
type Engine struct {
	host string
}

// Local returns an engine for the local Docker daemon.
func Local() *Engine { return &Engine{} }

// Remote returns an engine that runs Docker on sshTarget (e.g. "root@1.2.3.4") over SSH.
func Remote(sshTarget string) *Engine { return &Engine{host: "ssh://" + sshTarget} }

// For returns a remote engine when host is set, otherwise the local engine.
func For(host string) *Engine {
	if host == "" {
		return Local()
	}
	return Remote(host)
}

// IsRemote reports whether the engine targets a remote host.
func (e *Engine) IsRemote() bool { return e.host != "" }

// SSHHostname returns the host part of an ssh target ("root@1.2.3.4" → "1.2.3.4").
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

// Available checks that the docker CLI and daemon are reachable.
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

// BuildFromDockerfile builds image tag using the given Dockerfile contents, with
// contextDir as the build context, streaming output to out.
func (e *Engine) BuildFromDockerfile(ctx context.Context, tag, dockerfile, contextDir string, out io.Writer) error {
	defer ensureDockerignore(contextDir)()

	cmd := e.contextCommand(ctx, "build", "-t", tag, "-f", "-", contextDir)
	cmd.Stdin = strings.NewReader(dockerfile)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// RunSpec describes how to run an app container.
type RunSpec struct {
	Name          string
	Image         string
	ContainerPort int
	Memory        string // optional, e.g. "512m"
	CPU           string // optional, e.g. "0.5"
	Env           map[string]string
	Public        bool // publish on all interfaces instead of 127.0.0.1
}

// Run replaces any container named s.Name with a fresh one from s.Image,
// publishing the container port to a random host port. Returns that host port.
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

// HostPort returns the host port that containerPort is published on.
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

// State returns the container's status ("running", "exited", …) or "missing".
func (e *Engine) State(container string) string {
	out, err := e.command("inspect", "-f", "{{.State.Status}}", container).Output()
	if err != nil {
		return "missing"
	}
	return strings.TrimSpace(string(out))
}

// Containers returns the names of all Skiff-managed containers (running or not).
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

// Stop gracefully stops a container (SIGTERM, then SIGKILL after a grace period).
func (e *Engine) Stop(container string) error {
	out, err := e.command("stop", container).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out))
	}
	return nil
}

// Remove force-removes a container by name.
func (e *Engine) Remove(name string) error {
	out, err := e.command("rm", "-f", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out))
	}
	return nil
}

// Logs streams a container's logs to out. When follow is true it keeps
// streaming; tail limits how many recent lines are shown first.
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

// StreamLogs follows a container's recent logs to out until ctx is canceled.
func (e *Engine) StreamLogs(ctx context.Context, container string, out io.Writer) error {
	cmd := e.contextCommand(ctx, "logs", "--tail", "100", "--follow", container)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
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
