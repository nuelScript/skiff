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
	Name     string `toml:"name"`
	Replicas int    `toml:"replicas"`

	Server    Server            `toml:"server"`
	Build     Build             `toml:"build"`
	Deploy    Deploy            `toml:"deploy"`
	Resources Resources         `toml:"resources"`
	Env       map[string]string `toml:"env"`
	Secrets   map[string]string `toml:"secrets"` // runtime only, never baked into the image
}

type Deploy struct {
	Release string `toml:"release"`
	Network string `toml:"network"`
}

type Resources struct {
	Memory string `toml:"memory"`
	CPU    string `toml:"cpu"`
}

type Server struct {
	Host string `toml:"host"`
}

type Build struct {
	Dockerfile string `toml:"dockerfile"`
	Port       int    `toml:"port"`

	// Optional recipe overrides: setting start or static skips auto-detection and builds from these instead of a Dockerfile.
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
	if c.Replicas < 1 {
		c.Replicas = 1
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
