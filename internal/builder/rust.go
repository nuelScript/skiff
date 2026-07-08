package builder

import (
	"os"
	"path/filepath"
	"strings"
)

type rustBuilder struct{ dir string }

func (r *rustBuilder) Name() string { return "Rust" }

func (r *rustBuilder) detect() bool {
	return fileExists(filepath.Join(r.dir, "Cargo.toml"))
}

func (r *rustBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	bin := cargoPackageName(r.dir)
	if bin == "" {
		bin = "app"
	}
	return render(Plan{
		Base:        "rust:1-slim",
		Build:       []string{"cargo build --release"},
		Env:         env,
		RuntimeBase: "debian:stable-slim",
		Copy:        []Artifact{{From: "/app/target/release/" + bin, To: "/app/server"}},
		Start:       []string{"/app/server"},
		Port:        port,
	})
}

// cargoPackageName reads [package] name from Cargo.toml — the name of the built release binary.
func cargoPackageName(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if err != nil {
		return ""
	}
	inPackage := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inPackage = line == "[package]"
			continue
		}
		if inPackage && strings.HasPrefix(line, "name") {
			if i := strings.IndexByte(line, '"'); i >= 0 {
				if j := strings.IndexByte(line[i+1:], '"'); j >= 0 {
					return line[i+1 : i+1+j]
				}
			}
		}
	}
	return ""
}
