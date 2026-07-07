package docker

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (e *Engine) BuildFromDockerfile(ctx context.Context, tag, dockerfile, contextDir string, out io.Writer) error {
	defer ensureDockerignore(contextDir)()

	cmd := e.contextCommand(ctx, "build", "-t", tag, "-f", "-", contextDir)
	cmd.Stdin = strings.NewReader(dockerfile)
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
