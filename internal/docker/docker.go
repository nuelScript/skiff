// Package docker deploys apps to a Docker engine via the docker CLI.
package docker

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// Available checks that the docker CLI and daemon are reachable.
func Available() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found in PATH")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		return fmt.Errorf("docker daemon unreachable: %s", firstLine(out))
	}
	return nil
}

// BuildFromDockerfile builds image tag using the given Dockerfile contents,
// with contextDir as the build context, streaming output to out.
func BuildFromDockerfile(tag, dockerfile, contextDir string, out io.Writer) error {
	cmd := exec.Command("docker", "build", "-t", tag, "-f", "-", contextDir)
	cmd.Stdin = strings.NewReader(dockerfile)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Run replaces any container named name with a fresh one from image, publishing
// containerPort to a random host port on 127.0.0.1. Returns that host port.
func Run(name, image string, containerPort int) (int, error) {
	_ = exec.Command("docker", "rm", "-f", name).Run() // best-effort: drop the old one

	out, err := exec.Command("docker", "run", "-d",
		"--name", name,
		"--restart", "unless-stopped",
		"--label", "skiff=1",
		"-p", fmt.Sprintf("127.0.0.1::%d", containerPort),
		image,
	).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("docker run failed: %s", firstLine(out))
	}
	return HostPort(name, containerPort)
}

// HostPort returns the host port that containerPort is published on.
func HostPort(name string, containerPort int) (int, error) {
	out, err := exec.Command("docker", "port", name, strconv.Itoa(containerPort)).Output()
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

// Remove force-removes a container by name.
func Remove(name string) error {
	out, err := exec.Command("docker", "rm", "-f", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out))
	}
	return nil
}

// Logs streams a container's logs to out. When follow is true it keeps
// streaming; tail limits how many recent lines are shown first.
func Logs(container string, follow bool, tail string, out io.Writer) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "--follow")
	}
	if tail != "" {
		args = append(args, "--tail", tail)
	}
	args = append(args, container)
	cmd := exec.Command("docker", args...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

func firstLine(b []byte) string {
	if i := strings.IndexByte(string(b), '\n'); i >= 0 {
		return string(b[:i])
	}
	return strings.TrimSpace(string(b))
}
