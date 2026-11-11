package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A build's config keeps the server's other logins and plugins, and its own login isn't overridden by a credential helper.
func TestBuildConfigAddsTheLogin(t *testing.T) {
	own := t.TempDir()
	t.Setenv("DOCKER_CONFIG", own)
	_ = os.WriteFile(filepath.Join(own, "config.json"), []byte(`{"auths":{"ghcr.io":{"auth":"b2xk"}},"credsStore":"desktop","credHelpers":{"registry.example.com":"ecr-login"}}`), 0o600)
	_ = os.MkdirAll(filepath.Join(own, "cli-plugins"), 0o755)
	dir, err := buildConfigDir(RegistryAuth{Host: "registry.example.com", Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	var cfg struct {
		Auths       map[string]any    `json:"auths"`
		CredsStore  string            `json:"credsStore"`
		CredHelpers map[string]string `json:"credHelpers"`
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	_ = json.Unmarshal(b, &cfg)
	if cfg.Auths["ghcr.io"] == nil || cfg.Auths["registry.example.com"] == nil || cfg.CredsStore != "desktop" || cfg.CredHelpers["registry.example.com"] != "" {
		t.Errorf("config: %s", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "cli-plugins")); err != nil {
		t.Errorf("plugins not reachable: %v", err)
	}
}

// Other registries keep the server's credential helper, and the build config lives under the panel's own folder.
func TestBuildConfigKeepsOtherHelpers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	own := t.TempDir()
	t.Setenv("DOCKER_CONFIG", own)
	_ = os.WriteFile(filepath.Join(own, "config.json"), []byte(`{"credsStore":"desktop","credHelpers":{"other.io":"ecr-login"}}`), 0o600)
	dir, err := buildConfigDir(RegistryAuth{Host: "registry.example.com", Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if filepath.Dir(dir) != filepath.Join(home, ".skiff", "build-config") {
		t.Errorf("dir: %s", dir)
	}
	var cfg struct {
		CredsStore  string            `json:"credsStore"`
		CredHelpers map[string]string `json:"credHelpers"`
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	_ = json.Unmarshal(b, &cfg)
	if v, ok := cfg.CredHelpers["registry.example.com"]; cfg.CredsStore != "desktop" || cfg.CredHelpers["other.io"] != "ecr-login" || !ok || v != "" {
		t.Errorf("config: %s", b)
	}
}

func TestBuildConfigClearsOldLeftovers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".skiff", "build-config")
	old := filepath.Join(root, "build-old")
	_ = os.MkdirAll(old, 0o700)
	past := time.Now().Add(-24 * time.Hour)
	_ = os.Chtimes(old, past, past)
	dir, err := tempBuildDir()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if _, err := os.Stat(old); err == nil {
		t.Error("an old leftover should be removed")
	}
}
