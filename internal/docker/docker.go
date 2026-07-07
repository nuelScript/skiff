// Package docker deploys apps to a Docker engine, local or remote over SSH.
package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
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

// cmdErr turns a failed command's captured output into an error, falling back to
// the raw exit error when the command produced no output (e.g. an OOM-killed
// container exits 137 with nothing captured) so the error message is never blank.
func cmdErr(out []byte, err error) error {
	if msg := firstLine(out); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	return err
}

// splitLines turns command output into its non-empty, trimmed lines — the shared
// shape behind every `docker ps --format …` reader.
func splitLines(b []byte) []string {
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
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
