package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
)

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
