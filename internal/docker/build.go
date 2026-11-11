package docker

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// BuildOptions are the build arguments and Dockerfile stage a build asks for, and a registry login for its base image when that's private.
type BuildOptions struct {
	Args   map[string]string
	Target string
	Auth   *RegistryAuth
}

// BuildFromDockerfile builds contextDir into tag. With pull, every base image comes from its registry, so a build can't use images other apps left on this machine.
func (e *Engine) BuildFromDockerfile(ctx context.Context, tag, dockerfile, contextDir string, pull bool, opts BuildOptions, out io.Writer) error {
	defer ensureDockerignore(contextDir)()

	args := []string{"build", "-t", tag, "-f", "-"}
	if pull {
		args = append(args, "--pull")
	}
	for _, k := range slices.Sorted(maps.Keys(opts.Args)) {
		args = append(args, "--build-arg", k+"="+opts.Args[k])
	}
	if opts.Target != "" {
		args = append(args, "--target", opts.Target)
	}
	cmd := e.contextCommand(ctx, append(args, contextDir)...)
	if opts.Auth != nil && opts.Auth.Username != "" {
		dir, err := buildConfigDir(*opts.Auth)
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = append(cmd.Env, "DOCKER_CONFIG="+dir)
	}
	cmd.Stdin = strings.NewReader(dockerfile)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// skiff.toml holds an app's secrets, so it never goes into the image, even when a Dockerfile copies everything. The checkout's .git can hold the credentials it was cloned with.
const defaultDockerignore = "node_modules\n.git\n.skiff\n.env\n*.log\nskiff.toml\n"

// alwaysIgnored comes after an app's own rules, which can't re-include these.
const alwaysIgnored = "\n.git\nskiff.toml\n!.skiff-mounts\n!.skiff-mounts/**\n"

// ensureDockerignore keeps skiff.toml out of the build for the length of it: it writes a default file when the app ships none, or adds the line to the app's own, and its cleanup puts things back as they were.
func ensureDockerignore(dir string) func() {
	p := filepath.Join(dir, ".dockerignore")
	existing, err := os.ReadFile(p)
	if err != nil {
		if err := os.WriteFile(p, []byte(defaultDockerignore), 0o644); err != nil {
			return func() {}
		}
		return func() { os.Remove(p) }
	}
	if err := os.WriteFile(p, append(append([]byte{}, existing...), alwaysIgnored...), 0o644); err != nil {
		return func() {}
	}
	return func() { _ = os.WriteFile(p, existing, 0o644) }
}

// buildConfigDir is a Docker config for one build: the server's own, with the login added for its registry. The server's plugins stay reachable, since a build may need buildx from there.
func buildConfigDir(a RegistryAuth) (string, error) {
	own := os.Getenv("DOCKER_CONFIG")
	if own == "" {
		home, _ := os.UserHomeDir()
		own = filepath.Join(home, ".docker")
	}
	cfg := map[string]any{}
	if b, err := os.ReadFile(filepath.Join(own, "config.json")); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	var login map[string]any
	_ = json.Unmarshal(dockerAuthConfig(a), &login)
	auths, _ := cfg["auths"].(map[string]any)
	if auths == nil {
		auths = map[string]any{}
	}
	maps.Copy(auths, login["auths"].(map[string]any))
	cfg["auths"] = auths
	// A credential helper would answer for the registry ahead of the login written here; an empty entry sends just that registry to the login, and the rest keep the server's helpers.
	helpers, _ := cfg["credHelpers"].(map[string]any)
	if helpers == nil {
		helpers = map[string]any{}
	}
	for host := range login["auths"].(map[string]any) {
		helpers[host] = ""
	}
	cfg["credHelpers"] = helpers
	dir, err := tempBuildDir()
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	if _, err := os.Stat(filepath.Join(own, "cli-plugins")); err == nil {
		_ = os.Symlink(filepath.Join(own, "cli-plugins"), filepath.Join(dir, "cli-plugins"))
	}
	return dir, nil
}

// tempBuildDir keeps a build's config under the panel's own folder rather than a shared temp one, and clears what a killed build left behind.
func tempBuildDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".skiff", "build-config")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 6*time.Hour {
			os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
	return os.MkdirTemp(root, "build-")
}
