package cmd

import (
	"fmt"

	"github.com/nuelScript/skiff/internal/proxy"
	"github.com/nuelScript/skiff/internal/registry"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List deployed apps",
		RunE: func(cmd *cobra.Command, args []string) error {
			apps, err := registry.List()
			if err != nil {
				return err
			}

			ui.Banner(version)
			if len(apps) == 0 {
				ui.Note("No apps yet — run `skiff deploy`.")
				fmt.Println()
				return nil
			}
			for _, a := range apps {
				ui.Field(a.Name, fmt.Sprintf("%s  ·  http://localhost:%d", proxy.URL(a.Name), a.HostPort))
			}
			fmt.Println()
			return nil
		},
	}
}
