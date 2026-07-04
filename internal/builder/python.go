package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func (p *pythonBuilder) Dockerfile() (string, error) {
	install := ""
	switch {
	case fileExists(filepath.Join(p.dir, "requirements.txt")):
		install = "RUN pip install --no-cache-dir -r requirements.txt\n"
	case fileExists(filepath.Join(p.dir, "pyproject.toml")):
		install = "RUN pip install --no-cache-dir .\n"
	}

	cmd, err := p.cmd()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`FROM python:3-alpine
WORKDIR /app
COPY . .
%s%s
`, install, cmd), nil
}

// cmd returns the Dockerfile CMD in exec (JSON) form so the app gets OS signals
// directly. A Procfile "web:" line wins; otherwise a conventional entrypoint.
func (p *pythonBuilder) cmd() (string, error) {
	if web := procfileWeb(filepath.Join(p.dir, "Procfile")); web != "" {
		return `CMD ["sh", "-c", ` + strconv.Quote(web) + `]`, nil
	}
	for _, f := range []string{"main.py", "app.py", "server.py"} {
		if fileExists(filepath.Join(p.dir, f)) {
			return `CMD ["python", ` + strconv.Quote(f) + `]`, nil
		}
	}
	return "", fmt.Errorf("couldn't find a Python entrypoint (main.py/app.py/server.py) or a Procfile web: command")
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
