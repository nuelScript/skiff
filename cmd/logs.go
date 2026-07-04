package cmd

import (
	"fmt"
	"os"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/registry"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	var follow bool
	var tail string
	cmd := &cobra.Command{
		Use:   "logs <app>",
		Short: "Show logs for a deployed app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			apps, err := registry.Load()
			if err != nil {
				return err
			}
			app, ok := apps[name]
			if !ok {
				ui.Fail("No app named " + name + " — run `skiff ls`")
				return fmt.Errorf("unknown app %q", name)
			}
			return docker.For(app.Host).Logs(app.Container, follow, tail, os.Stdout)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new logs as they arrive")
	cmd.Flags().StringVarP(&tail, "tail", "n", "100", "number of recent lines to show first")
	return cmd
}
