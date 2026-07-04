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
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Build and deploy the current app to your server",
		Long: `Deploy builds your app, runs it, and serves it at a URL.
Reads config from skiff.toml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(configPath, timeout, buildTimeout)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", config.DefaultFile, "path to skiff.toml")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultHealthTimeout, "how long to wait for the new version to become healthy")
	cmd.Flags().DurationVar(&buildTimeout, "build-timeout", defaultBuildTimeout, "cancel the build if it runs longer than this")
	return cmd
}

func runDeploy(configPath string, timeout, buildTimeout time.Duration) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		ui.Fail("Couldn't load config")
		return err
	}
	if !cfg.IsLocal() {
		ui.Fail("Remote targets aren't wired up yet — omit [server] to use local Docker")
		return fmt.Errorf("remote target %q not supported yet", cfg.Server.Host)
	}

	ui.Banner(version)
	fmt.Println("  " + ui.Accent("Deploying "+cfg.Name))
	fmt.Println()

	contextDir := filepath.Dir(configPath)
	b, err := builder.Select(contextDir, cfg.Build.Dockerfile)
	if err != nil {
		ui.Fail(err.Error())
		return err
	}

	ui.Field("target", cfg.TargetLabel())
	ui.Field("build", b.Name())
	fmt.Println()

	if err := docker.Available(); err != nil {
		ui.Fail(err.Error())
		return err
	}

	start := time.Now()
	image := fmt.Sprintf("skiff-%s:latest", cfg.Name)

	dockerfile, err := b.Dockerfile(cfg.Build.Port)
	if err != nil {
		ui.Fail("Couldn't prepare the build")
		return err
	}

	// Cancel the build on Ctrl-C or after buildTimeout, so a hung or runaway
	// build never stalls forever.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if buildTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, buildTimeout)
		defer cancel()
	}

	ui.Step("Building " + image)
	if err := docker.BuildFromDockerfile(ctx, image, dockerfile, contextDir, os.Stdout); err != nil {
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

	apps, err := registry.Load()
	if err != nil {
		ui.Fail("Couldn't read the app registry")
		return err
	}
	previous, hadPrevious := apps[cfg.Name]

	// Start the new version alongside the current one, under its own name.
	container := fmt.Sprintf("%s-%s", cfg.Name, shortID())
	ui.Step("Starting new version")
	hostPort, err := docker.Run(docker.RunSpec{
		Name:          container,
		Image:         image,
		ContainerPort: cfg.Build.Port,
		Memory:        cfg.Resources.Memory,
		CPU:           cfg.Resources.CPU,
	})
	if err != nil {
		ui.Fail("Couldn't start container")
		return err
	}

	ui.Step("Waiting for it to be healthy")
	if err := waitHealthy(hostPort, timeout); err != nil {
		_ = docker.Remove(container) // roll back; the old version keeps serving
		ui.Fail("New version never became healthy — rolled back")
		return err
	}
	ui.Done("Healthy")

	// Point the router at the new version (atomic), then retire the old one.
	if err := registry.Put(registry.App{
		Name:      cfg.Name,
		Container: container,
		Port:      cfg.Build.Port,
		HostPort:  hostPort,
	}); err != nil {
		ui.Fail("Couldn't update the registry")
		return err
	}

	if hadPrevious && previous.Container != "" && previous.Container != container {
		_ = docker.Stop(previous.Container) // graceful drain (SIGTERM) before removal
		_ = docker.Remove(previous.Container)
		ui.Done("Retired the previous version")
	}

	fmt.Println()
	ui.Live(proxy.URL(cfg.Name), time.Since(start))
	ui.Field("direct", fmt.Sprintf("http://localhost:%d", hostPort))
	return nil
}

func shortID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000)
	}
	return hex.EncodeToString(b)
}

func waitHealthy(hostPort int, timeout time.Duration) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/", hostPort)
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
