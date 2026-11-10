package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
	if cfg.Auths["ghcr.io"] == nil || cfg.Auths["registry.example.com"] == nil || cfg.CredsStore != "" || cfg.CredHelpers["registry.example.com"] != "" {
		t.Errorf("config: %s", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "cli-plugins")); err != nil {
		t.Errorf("plugins not reachable: %v", err)
	}
}
