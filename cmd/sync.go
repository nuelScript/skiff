package cmd

import (
	"fmt"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/registry"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Reconcile containers with the registry (prune orphans and dead entries)",
		RunE: func(cmd *cobra.Command, args []string) error {
			apps, err := registry.List()
			if err != nil {
				return err
			}

			ui.Banner(version)

			expected := map[string]bool{}
			for _, a := range apps {
				expected[a.Container] = true
			}

			local := docker.Local()
			containers, err := local.Containers()
			if err != nil {
				return err
			}
			changed := 0
			for _, c := range containers {
				if !expected[c] {
					if err := local.Remove(c); err == nil {
						ui.Done("Pruned orphan container " + c)
						changed++
					}
				}
			}

			for _, a := range apps {
				if docker.For(a.Host).State(a.Container) == "missing" {
					if ok, _ := registry.Delete(a.Name); ok {
						ui.Done("Dropped dead app " + a.Name)
						changed++
					}
				}
			}

			if changed == 0 {
				ui.Note("Everything's in sync.")
			}
			fmt.Println()
			return nil
		},
	}
}
