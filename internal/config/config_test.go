package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDefaults(t *testing.T) {
	p := write(t, "skiff.toml", "name = \"api\"\n")
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Name != "api" {
		t.Errorf("name = %q", c.Name)
	}
	if c.Build.Port != 8080 {
		t.Errorf("default port = %d, want 8080", c.Build.Port)
	}
	if c.Replicas != 1 {
		t.Errorf("default replicas = %d, want 1", c.Replicas)
	}
	if c.Build.Dockerfile != "Dockerfile" {
		t.Errorf("default dockerfile = %q", c.Build.Dockerfile)
	}
}

func TestLoadRequiresName(t *testing.T) {
	p := write(t, "skiff.toml", "replicas = 2\n")
	if _, err := Load(p); err == nil {
		t.Fatal("Load accepted a config with no name")
	}
}

func TestLoadValues(t *testing.T) {
	p := write(t, "skiff.toml", `
name = "web"
replicas = 4
[build]
port = 3000
[deploy]
release = "npm run migrate"
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Build.Port != 3000 || c.Replicas != 4 || c.Deploy.Release != "npm run migrate" {
		t.Fatalf("parsed wrong: %+v", c)
	}
}
