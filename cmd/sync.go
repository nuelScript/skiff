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

			// 1) Remove Skiff containers the registry doesn't track (orphans).
			containers, err := docker.Containers()
			if err != nil {
				return err
			}
			changed := 0
			for _, c := range containers {
				if !expected[c] {
					if err := docker.Remove(c); err == nil {
						ui.Done("Pruned orphan container " + c)
						changed++
					}
				}
			}

			// 2) Drop registry entries whose container is gone.
			for _, a := range apps {
				if docker.State(a.Container) == "missing" {
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
