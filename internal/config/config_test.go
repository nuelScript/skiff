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

func TestEnvironmentMerge(t *testing.T) {
	dir := t.TempDir()
	dotenv := "# comment\n\nexport FOO=from_dotenv\nBAR=\"quoted\"\nBAZ='single'\nOVERRIDE=dotenv\nnoequals\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Config{Env: map[string]string{"OVERRIDE": "config", "NEW": "x"}}
	env := c.Environment(dir)

	want := map[string]string{
		"FOO":      "from_dotenv", // "export " prefix stripped
		"BAR":      "quoted",      // double quotes trimmed
		"BAZ":      "single",      // single quotes trimmed
		"OVERRIDE": "config",      // c.Env wins over the .env
		"NEW":      "x",           // config-only var present
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("env[%q] = %q, want %q", k, env[k], v)
		}
	}
	if _, ok := env["noequals"]; ok {
		t.Error("a line without '=' should be skipped")
	}
}

func TestEnvironmentNoDotenv(t *testing.T) {
	c := &Config{Env: map[string]string{"A": "1"}}
	env := c.Environment(t.TempDir()) // no .env present
	if len(env) != 1 || env["A"] != "1" {
		t.Errorf("missing .env should yield just the config env, got %v", env)
	}
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
