// Package config loads and validates a skiff.toml describing a single app.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const DefaultFile = "skiff.toml"

type Config struct {
	Name string `toml:"name"`

	Server    ServerConfig      `toml:"server"`
	Build     BuildConfig       `toml:"build"`
	Resources ResourcesConfig   `toml:"resources"`
	Env       map[string]string `toml:"env"`     // available at build + runtime
	Secrets   map[string]string `toml:"secrets"` // runtime only (never baked into the image)
}

type ResourcesConfig struct {
	Memory string `toml:"memory"` // e.g. "512m"
	CPU    string `toml:"cpu"`    // e.g. "0.5"
}

// ServerConfig describes where the app runs. An empty host means local Docker.
type ServerConfig struct {
	Host string `toml:"host"`
}

type BuildConfig struct {
	Dockerfile string `toml:"dockerfile"`
	Port       int    `toml:"port"`

	// Optional recipe overrides — set start or static to skip auto-detection
	// and build from these instead of a Dockerfile.
	Base    string `toml:"base"`
	Install string `toml:"install"`
	Build   string `toml:"build"`
	Start   string `toml:"start"`
	Static  string `toml:"static"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Build.Dockerfile == "" {
		c.Build.Dockerfile = "Dockerfile"
	}
	if c.Build.Port == 0 {
		c.Build.Port = 8080
	}
}

func (c *Config) validate() error {
	if c.Name == "" {
		return fmt.Errorf("skiff.toml is missing required field: name")
	}
	return nil
}

func (c *Config) IsLocal() bool {
	return c.Server.Host == "" || c.Server.Host == "local"
}

func (c *Config) TargetLabel() string {
	if c.IsLocal() {
		return "local docker"
	}
	return c.Server.Host
}

func (c *Config) RemoteHost() string {
	if c.IsLocal() {
		return ""
	}
	return c.Server.Host
}

func (c *Config) Environment(dir string) map[string]string {
	env := loadDotenv(filepath.Join(dir, ".env"))
	for k, v := range c.Env {
		env[k] = v
	}
	return env
}

func loadDotenv(path string) map[string]string {
	env := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return env
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" {
			env[k] = v
		}
	}
	return env
}
