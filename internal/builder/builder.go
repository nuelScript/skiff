// Package builder turns app source into a Dockerfile, choosing a strategy from
// what's in the app directory: the app's own Dockerfile, or a detected stack.
package builder

import (
	"fmt"
	"os"
	"path/filepath"
)

// Builder produces a Dockerfile for an app directory.
type Builder interface {
	// Name is a short label for the strategy, e.g. "Node.js".
	Name() string
	// Dockerfile returns the Dockerfile contents to build the app with.
	Dockerfile() (string, error)
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

// stacks lists the stack builders in priority order. Supporting a new
// language/framework is one more entry here.
func stacks(dir string) []stackBuilder {
	return []stackBuilder{
		&nodeBuilder{dir: dir},
	}
}

type stackBuilder interface {
	Builder
	detect() bool
}

// dockerfileBuilder uses the app's own Dockerfile.
type dockerfileBuilder struct{ path string }

func (d *dockerfileBuilder) Name() string { return "Dockerfile" }

func (d *dockerfileBuilder) Dockerfile() (string, error) {
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
