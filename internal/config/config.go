// Package config loads and validates a skiff.toml describing a single app.
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// DefaultFile is the config filename looked up in the current directory.
const DefaultFile = "skiff.toml"

// Config is a parsed skiff.toml.
type Config struct {
	Name string `toml:"name"`

	Server ServerConfig `toml:"server"`
	Build  BuildConfig  `toml:"build"`
}

// ServerConfig describes where the app runs. An empty host means local Docker.
type ServerConfig struct {
	Host string `toml:"host"`
}

// BuildConfig describes how the app image is built and served.
type BuildConfig struct {
	Dockerfile string `toml:"dockerfile"`
	Port       int    `toml:"port"`
}

// Load reads, defaults, and validates a skiff.toml from path.
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

// IsLocal reports whether the app deploys to the local Docker engine.
func (c *Config) IsLocal() bool {
	return c.Server.Host == "" || c.Server.Host == "local"
}

// TargetLabel is a short human label for the deploy target.
func (c *Config) TargetLabel() string {
	if c.IsLocal() {
		return "local docker"
	}
	return c.Server.Host
}
