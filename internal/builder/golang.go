package builder

import (
	"os"
	"path/filepath"
	"strings"
)

// goBuilder builds a Go app, detected by its go.mod, into a tiny runtime image
// via a multi-stage build.
type goBuilder struct{ dir string }

func (g *goBuilder) Name() string { return "Go" }

func (g *goBuilder) detect() bool {
	return fileExists(filepath.Join(g.dir, "go.mod"))
}

func (g *goBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	cache := []string{"go.mod"}
	if fileExists(filepath.Join(g.dir, "go.sum")) {
		cache = append(cache, "go.sum")
	}
	return render(Plan{
		Base:        g.baseImage(),
		CacheFiles:  cache,
		Install:     []string{"go mod download"},
		Build:       []string{"CGO_ENABLED=0 go build -o /server ."},
		Env:         env,
		RuntimeBase: "alpine:3.20",
		Copy:        []Artifact{{From: "/server", To: "/server"}},
		Start:       []string{"/server"},
		Port:        port,
	})
}

func (g *goBuilder) baseImage() string {
	if v := goVersion(g.dir); v != "" {
		return "golang:" + v
	}
	return "golang:1.23"
}

// goVersion reads the "go X.Y" directive from go.mod and returns "X.Y".
func goVersion(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "go "))
			if parts := strings.Split(v, "."); len(parts) >= 2 {
				return parts[0] + "." + parts[1]
			}
		}
	}
	return ""
}
