// Package registry tracks the apps Skiff has deployed locally, in ~/.skiff/apps.json.
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// App is one deployed app.
type App struct {
	Name      string `json:"name"`
	Container string `json:"container"`
	Port      int    `json:"port"`           // port the app listens on inside the container
	HostPort  int    `json:"hostPort"`       // host port the container is published on
	Host      string `json:"host,omitempty"` // remote ssh target ("" = local docker)
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".skiff")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func file() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "apps.json"), nil
}

// Load returns all known apps keyed by name.
func Load() (map[string]App, error) {
	f, err := file()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(f)
	if os.IsNotExist(err) {
		return map[string]App{}, nil
	}
	if err != nil {
		return nil, err
	}
	apps := map[string]App{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &apps); err != nil {
			return nil, err
		}
	}
	return apps, nil
}

func save(apps map[string]App) error {
	f, err := file()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically so a concurrent reader never sees a half-written file.
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f)
}

// Put inserts or updates an app.
func Put(a App) error {
	apps, err := Load()
	if err != nil {
		return err
	}
	apps[a.Name] = a
	return save(apps)
}

// Delete removes an app, reporting whether it existed.
func Delete(name string) (bool, error) {
	apps, err := Load()
	if err != nil {
		return false, err
	}
	if _, ok := apps[name]; !ok {
		return false, nil
	}
	delete(apps, name)
	return true, save(apps)
}

// List returns all apps sorted by name.
func List() ([]App, error) {
	apps, err := Load()
	if err != nil {
		return nil, err
	}
	out := make([]App, 0, len(apps))
	for _, a := range apps {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
