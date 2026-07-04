package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pythonBuilder builds a Python app, detected by its project/entrypoint files.
type pythonBuilder struct{ dir string }

func (p *pythonBuilder) Name() string { return "Python" }

func (p *pythonBuilder) detect() bool {
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile", "main.py", "app.py", "server.py"} {
		if fileExists(filepath.Join(p.dir, f)) {
			return true
		}
	}
	return false
}

func (p *pythonBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	var install []string
	switch {
	case fileExists(filepath.Join(p.dir, "requirements.txt")):
		install = []string{"pip install --no-cache-dir -r requirements.txt"}
	case fileExists(filepath.Join(p.dir, "pyproject.toml")):
		install = []string{"pip install --no-cache-dir ."}
	}

	start, err := p.start()
	if err != nil {
		return "", err
	}

	return render(Plan{
		Base:    "python:3-alpine",
		Install: install,
		Env:     env,
		Start:   start,
		Port:    port,
	})
}

// start picks the argv to run: a Procfile "web:" line if present, otherwise a
// conventional entrypoint file.
func (p *pythonBuilder) start() ([]string, error) {
	if web := procfileWeb(filepath.Join(p.dir, "Procfile")); web != "" {
		return []string{"sh", "-c", web}, nil
	}
	for _, f := range []string{"main.py", "app.py", "server.py"} {
		if fileExists(filepath.Join(p.dir, f)) {
			return []string{"python", f}, nil
		}
	}
	return nil, fmt.Errorf("couldn't find a Python entrypoint (main.py/app.py/server.py) or a Procfile web: command")
}

func procfileWeb(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "web:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "web:"))
		}
	}
	return ""
}
