package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Build and deploy the current app to your server",
		Long: `Deploy builds your app, runs it, and serves it at a URL.
Reads config from skiff.toml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(configPath)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", config.DefaultFile, "path to skiff.toml")
	return cmd
}

func runDeploy(configPath string) error {
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
	ui.Field("target", cfg.TargetLabel())
	ui.Field("build", cfg.Build.Dockerfile)
	ui.Field("port", fmt.Sprintf("%d", cfg.Build.Port))
	fmt.Println()

	if err := docker.Available(); err != nil {
		ui.Fail(err.Error())
		return err
	}

	start := time.Now()

	contextDir := filepath.Dir(configPath)
	dockerfile := filepath.Join(contextDir, cfg.Build.Dockerfile)
	image := fmt.Sprintf("skiff-%s:latest", cfg.Name)

	ui.Step("Building " + image)
	if err := docker.Build(image, dockerfile, contextDir, os.Stdout); err != nil {
		ui.Fail("Build failed")
		return err
	}
	ui.Done("Built " + image)

	ui.Step("Starting container")
	if err := docker.Run(cfg.Name, image, cfg.Build.Port, cfg.Build.Port); err != nil {
		ui.Fail("Couldn't start container")
		return err
	}
	ui.Done("Started " + cfg.Name)

	url := fmt.Sprintf("http://localhost:%d", cfg.Build.Port)
	fmt.Println()
	ui.Live(url, time.Since(start))
	return nil
}
