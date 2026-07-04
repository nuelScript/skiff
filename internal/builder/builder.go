// Package builder turns app source into a Dockerfile by detecting the stack.
package builder

import (
	"fmt"
	"os"
	"path/filepath"
)

type Builder interface {
	Name() string
	Dockerfile(port int, env map[string]string) (string, error)
}

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
		&rustBuilder{dir: dir},
		&rubyBuilder{dir: dir},
		&elixirBuilder{dir: dir},
		&javaBuilder{dir: dir},
		&dotnetBuilder{dir: dir},
		&phpBuilder{dir: dir},
		&staticBuilder{dir: dir},
	}
}

type stackBuilder interface {
	Builder
	detect() bool
}

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
