package builder

import (
	"path/filepath"
	"strconv"
)

// phpBuilder builds a plain PHP app (detected by an index.php) using PHP's
// built-in web server.
type phpBuilder struct{ dir string }

func (p *phpBuilder) Name() string { return "PHP" }

func (p *phpBuilder) detect() bool {
	return fileExists(filepath.Join(p.dir, "index.php")) ||
		fileExists(filepath.Join(p.dir, "public", "index.php"))
}

func (p *phpBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	docroot := "."
	if fileExists(filepath.Join(p.dir, "public", "index.php")) {
		docroot = "public"
	}
	return render(Plan{
		Base:  "php:8.3-cli",
		Env:   env,
		Start: []string{"php", "-S", "0.0.0.0:" + strconv.Itoa(port), "-t", docroot},
		Port:  port,
	})
}
