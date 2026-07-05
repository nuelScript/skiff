package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/nuelScript/skiff/internal/builder"
	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/proxy"
	"github.com/nuelScript/skiff/internal/registry"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

const (
	defaultHealthTimeout = 30 * time.Second
	defaultBuildTimeout  = 15 * time.Minute
)

func newDeployCmd() *cobra.Command {
	var configPath string
	var timeout time.Duration
	var buildTimeout time.Duration
	var name string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Build and deploy the current app to your server",
		Long: `Deploy builds your app, runs it, and serves it at a URL.
Reads config from skiff.toml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(configPath, timeout, buildTimeout, name)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", config.DefaultFile, "path to skiff.toml")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultHealthTimeout, "how long to wait for the new version to become healthy")
	cmd.Flags().DurationVar(&buildTimeout, "build-timeout", defaultBuildTimeout, "cancel the build if it runs longer than this")
	cmd.Flags().StringVar(&name, "name", "", "override the app name (e.g. for preview environments)")
	return cmd
}

func runDeploy(configPath string, timeout, buildTimeout time.Duration, nameOverride string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		ui.Fail("Couldn't load config")
		return err
	}
	if nameOverride != "" {
		cfg.Name = nameOverride
	}

	// Local Docker, or a remote engine over SSH when [server] host is set.
	eng := docker.For(cfg.RemoteHost())

	ui.Banner(version)
	fmt.Println("  " + ui.Accent("Deploying "+cfg.Name))
	fmt.Println()

	contextDir := filepath.Dir(configPath)
	var b builder.Builder
	if bc := cfg.Build; bc.Start != "" || bc.Static != "" {
		b = builder.Custom(bc.Base, bc.Install, bc.Build, bc.Start, bc.Static)
	} else {
		b, err = builder.Select(contextDir, cfg.Build.Dockerfile)
		if err != nil {
			ui.Fail(err.Error())
			return err
		}
	}

	ui.Field("target", cfg.TargetLabel())
	ui.Field("build", b.Name())
	fmt.Println()

	if err := eng.Available(); err != nil {
		ui.Fail(err.Error())
		return err
	}

	start := time.Now()
	image := fmt.Sprintf("skiff-%s:latest", cfg.Name)

	buildEnv := cfg.Environment(contextDir)
	dockerfile, err := b.Dockerfile(cfg.Build.Port, buildEnv)
	if err != nil {
		ui.Fail("Couldn't prepare the build")
		return err
	}

	// Cancel the build on Ctrl-C or after buildTimeout.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if buildTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, buildTimeout)
		defer cancel()
	}

	ui.Step("Building " + image)
	if err := eng.BuildFromDockerfile(ctx, image, dockerfile, contextDir, os.Stdout); err != nil {
		switch ctx.Err() {
		case context.DeadlineExceeded:
			ui.Fail(fmt.Sprintf("Build exceeded %s — canceled", buildTimeout))
		case context.Canceled:
			ui.Fail("Build canceled")
		default:
			ui.Fail("Build failed")
		}
		return err
	}
	ui.Done("Built " + image)

	if err := releaseImage(eng, cfg, image, contextDir, timeout, start); err != nil {
		return err
	}

	// Retain this build as a rollback point (tagged by deploy id), pruning old ones.
	retainRollbackImage(eng, cfg.Name, image)
	return nil
}

// releaseImage runs an already-built image as a new container alongside the
// current one, waits for it to become healthy, atomically points the router at
// it, and retires the previous version. Shared by `deploy` (after a build) and
// `rollback` (re-running a retained image with no rebuild).
func releaseImage(eng *docker.Engine, cfg *config.Config, image, contextDir string, timeout time.Duration, start time.Time) error {
	// Runtime env = build env + secrets. Secrets never bake into the image, so a
	// rollback picks up the current secrets/env against the old code image.
	buildEnv := cfg.Environment(contextDir)
	runtimeEnv := make(map[string]string, len(buildEnv)+len(cfg.Secrets))
	for k, v := range buildEnv {
		runtimeEnv[k] = v
	}
	for k, v := range cfg.Secrets {
		runtimeEnv[k] = v
	}

	apps, err := registry.Load()
	if err != nil {
		ui.Fail("Couldn't read the app registry")
		return err
	}
	previous, hadPrevious := apps[cfg.Name]

	container := fmt.Sprintf("%s-%s", cfg.Name, shortID())
	ui.Step("Starting new version")
	hostPort, err := eng.Run(docker.RunSpec{
		Name:          container,
		App:           cfg.Name,
		Image:         image,
		ContainerPort: cfg.Build.Port,
		Memory:        cfg.Resources.Memory,
		CPU:           cfg.Resources.CPU,
		Env:           runtimeEnv,
		Public:        eng.IsRemote(),
	})
	if err != nil {
		ui.Fail("Couldn't start container")
		return err
	}

	healthHost := "127.0.0.1"
	if eng.IsRemote() {
		healthHost = docker.SSHHostname(cfg.Server.Host)
	}
	ui.Step("Waiting for it to be healthy")
	if err := waitHealthy(healthHost, hostPort, timeout); err != nil {
		_ = eng.Remove(container) // roll back; the old version keeps serving
		ui.Fail("New version never became healthy — rolled back")
		ui.Note(fmt.Sprintf("the app must listen on 0.0.0.0:%d (the `port` in skiff.toml) and answer HTTP", cfg.Build.Port))
		return err
	}
	ui.Done("Healthy")

	// Point the router at the new version (atomic), then retire the old one.
	if err := registry.Put(registry.App{
		Name:      cfg.Name,
		Container: container,
		Port:      cfg.Build.Port,
		HostPort:  hostPort,
		Host:      cfg.RemoteHost(),
	}); err != nil {
		ui.Fail("Couldn't update the registry")
		return err
	}

	if hadPrevious && previous.Container != "" && previous.Container != container {
		_ = eng.Stop(previous.Container) // graceful drain (SIGTERM) before removal
		_ = eng.Remove(previous.Container)
		ui.Done("Retired the previous version")
	}

	fmt.Println()
	if eng.IsRemote() {
		ui.Live(fmt.Sprintf("http://%s:%d", healthHost, hostPort), time.Since(start))
	} else {
		ui.Live(proxy.URL(cfg.Name), time.Since(start))
		ui.Field("direct", fmt.Sprintf("http://localhost:%d", hostPort))
	}
	return nil
}

// retainImages is how many past builds to keep as instant-rollback points, per app.
const retainImages = 5

// retainRollbackImage tags the just-built image by its deploy id so a later
// rollback can re-run it without a rebuild, then prunes builds beyond the last
// retainImages. Only runs when invoked by the panel (which sets SKIFF_DEPLOY_ID).
func retainRollbackImage(eng *docker.Engine, app, image string) {
	did := os.Getenv("SKIFF_DEPLOY_ID")
	if did == "" {
		return
	}
	if err := eng.Tag(image, fmt.Sprintf("skiff-%s:%s", app, did)); err != nil {
		return
	}
	tags := eng.AppImageTags(app) // newest first
	for i := retainImages; i < len(tags); i++ {
		_ = eng.RemoveImage(fmt.Sprintf("skiff-%s:%s", app, tags[i]))
	}
}

func shortID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000)
	}
	return hex.EncodeToString(b)
}

func waitHealthy(host string, hostPort int, timeout time.Duration) error {
	url := fmt.Sprintf("http://%s:%d/", host, hostPort)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("no response within %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("no response within %s", timeout)
}
