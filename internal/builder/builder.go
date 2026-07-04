// Package builder turns app source into a Dockerfile. It detects the stack and
// emits a Plan (base image, install, build, how to serve), which render() turns
// into a Dockerfile. The app's own Dockerfile is the escape hatch.
package builder

import (
	"fmt"
	"os"
	"path/filepath"
)

// Builder produces a Dockerfile for an app directory, serving on port.
type Builder interface {
	Name() string
	Dockerfile(port int, env map[string]string) (string, error)
}

// Select picks a builder for the app in dir: its own Dockerfile if present,
// otherwise the first stack whose files are detected.
func Select(dir, dockerfile string) (Builder, error) {
	if p := filepath.Join(dir, dockerfile); fileExists(p) {
		return &dockerfileBuilder{path: p}, nil
	}
	for _, b := range stacks(dir) {
		if b.detect() {
			return b, nil
		}
	}
	return nil, fmt.Errorf("couldn't detect how to build this app — add a Dockerfile")
}

// stacks lists the stack builders in priority order. Static is last so a runtime
// (which may also ship an index.html) wins over a plain static site.
func stacks(dir string) []stackBuilder {
	return []stackBuilder{
		&nodeBuilder{dir: dir},
		&pythonBuilder{dir: dir},
		&goBuilder{dir: dir},
		&phpBuilder{dir: dir},
		&staticBuilder{dir: dir},
	}
}

type stackBuilder interface {
	Builder
	detect() bool
}

// dockerfileBuilder uses the app's own Dockerfile.
type dockerfileBuilder struct{ path string }

func (d *dockerfileBuilder) Name() string { return "Dockerfile" }

func (d *dockerfileBuilder) Dockerfile(int, map[string]string) (string, error) {
	data, err := os.ReadFile(d.path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
