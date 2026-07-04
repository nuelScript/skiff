// Package docker deploys apps to a Docker engine via the docker CLI.
package docker

import (
	"fmt"
	"io"
	"os/exec"
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

// Build builds image tag from dockerfile using contextDir as the build context,
// streaming build output to out.
func Build(tag, dockerfile, contextDir string, out io.Writer) error {
	cmd := exec.Command("docker", "build", "-t", tag, "-f", dockerfile, contextDir)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Run replaces any container named name with a fresh one from image,
// publishing hostPort to containerPort.
func Run(name, image string, hostPort, containerPort int) error {
	_ = exec.Command("docker", "rm", "-f", name).Run() // best-effort: remove the old one

	out, err := exec.Command("docker", "run", "-d",
		"--name", name,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:%d", hostPort, containerPort),
		image,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %s", firstLine(out))
	}
	return nil
}

func firstLine(b []byte) string {
	if i := strings.IndexByte(string(b), '\n'); i >= 0 {
		return string(b[:i])
	}
	return strings.TrimSpace(string(b))
}
