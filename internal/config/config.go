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
	Name   string `toml:"name"`   // app name; used for the container + subdomain
	Domain string `toml:"domain"` // where the app is served (HTTPS is automatic)

	Server ServerConfig `toml:"server"`
	Build  BuildConfig  `toml:"build"`
}

// ServerConfig describes the machine Skiff deploys to.
type ServerConfig struct {
	Host string `toml:"host"` // ssh target, e.g. "root@203.0.113.10"
}

// BuildConfig describes how the app image is built and served.
type BuildConfig struct {
	Dockerfile string `toml:"dockerfile"` // path to the Dockerfile (default "Dockerfile")
	Port       int    `toml:"port"`       // port the app listens on inside the container
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
	var missing []string
	if c.Name == "" {
		missing = append(missing, "name")
	}
	if c.Domain == "" {
		missing = append(missing, "domain")
	}
	if c.Server.Host == "" {
		missing = append(missing, "server.host")
	}
	if len(missing) > 0 {
		return fmt.Errorf("skiff.toml is missing required field(s): %v", missing)
	}
	return nil
}
