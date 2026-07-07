package builder

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSelectNode(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "package.json")
	b, err := Select(dir, "Dockerfile")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if b.Name() != "Node.js" {
		t.Errorf("detected %q, want Node.js", b.Name())
	}
}

// A committed Dockerfile takes precedence over stack auto-detection.
func TestSelectDockerfileWins(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "package.json") // would otherwise be Node.js
	touch(t, dir, "Dockerfile")
	b, err := Select(dir, "Dockerfile")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if b.Name() != "Dockerfile" {
		t.Errorf("detected %q, want Dockerfile to win", b.Name())
	}
}

func TestSelectUndetected(t *testing.T) {
	if _, err := Select(t.TempDir(), "Dockerfile"); err == nil {
		t.Fatal("Select found a builder for an empty dir")
	}
}
