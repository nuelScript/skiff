package docker

import (
	"fmt"
	"net"
	"os/exec"
	"time"
)

// Forward opens an ssh tunnel from a free local port to 127.0.0.1:remotePort on sshTarget, so an app that listens only on its server's loopback can still be reached from here. Call stop when done.
func Forward(sshTarget string, remotePort int) (localPort int, stop func(), err error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	localPort = l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cmd := exec.Command("ssh", "-N", "-T",
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=30", "-o", "ExitOnForwardFailure=yes",
		"-L", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", localPort, remotePort),
		"--", sshTarget)
	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("opening a tunnel to %s: %w", sshTarget, err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	stop = func() {
		_ = cmd.Process.Kill()
		<-exited
	}
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return 0, nil, fmt.Errorf("couldn't open a tunnel to %s over SSH", sshTarget)
		default:
		}
		if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 500*time.Millisecond); err == nil {
			_ = c.Close()
			return localPort, stop, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	stop()
	return 0, nil, fmt.Errorf("timed out opening a tunnel to %s", sshTarget)
}
