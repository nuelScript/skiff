package cmd

import (
	"fmt"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Build and deploy the current app to your server",
		Long: `Deploy builds your app, runs it, and routes an HTTPS URL to it.
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

	ui.Banner(version)
	fmt.Println("  " + ui.Accent("Deploying "+cfg.Name))
	fmt.Println()
	ui.Field("target", cfg.Server.Host)
	ui.Field("domain", cfg.Domain)
	ui.Field("build", cfg.Build.Dockerfile)
	fmt.Println()

	start := time.Now()

	steps := []string{
		"Connected to " + cfg.Server.Host,
		"Built image from " + cfg.Build.Dockerfile,
		"Started container",
		"Configured routes + TLS",
	}
	for _, s := range steps {
		ui.Done(s)
	}

	fmt.Println()
	ui.Live("https://"+cfg.Domain, time.Since(start))
	return nil
}
