// Package registry tracks the apps Skiff has deployed locally, in ~/.skiff/apps.json.
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
)

type App struct {
	Name      string    `json:"name"`
	Container string    `json:"container"`        // representative replica (Replicas[0]), for status + back-compat
	Port      int       `json:"port"`             // port the app listens on inside the container
	HostPort  int       `json:"hostPort"`         // representative replica's published host port
	Host      string    `json:"host,omitempty"`   // remote ssh target ("" = local docker)
	Replicas  []Replica `json:"replicas,omitempty"` // every running container for the app
}

// Replica is one running container of an app.
type Replica struct {
	Container string `json:"container"`
	HostPort  int    `json:"hostPort"`
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

// mu serializes registry mutations across in-process goroutines; the file lock
// (below) additionally serializes against the `skiff deploy`/`rollback`
// subprocesses that write the same apps.json.
var mu sync.Mutex

// Update runs fn against the current registry under an exclusive lock (process
// mutex + an flock on apps.lock) and persists the result — the only safe way to
// read-modify-write, since deploy, autoscale, and teardown mutate concurrently
// from several goroutines and separate OS processes.
func Update(fn func(apps map[string]App)) error {
	mu.Lock()
	defer mu.Unlock()

	d, err := dir()
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(d, "apps.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	apps, err := Load()
	if err != nil {
		return err
	}
	fn(apps)
	return save(apps)
}

func Put(a App) error {
	return Update(func(apps map[string]App) { apps[a.Name] = a })
}

func Delete(name string) (bool, error) {
	existed := false
	err := Update(func(apps map[string]App) {
		if _, ok := apps[name]; ok {
			existed = true
			delete(apps, name)
		}
	})
	return existed, err
}

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
