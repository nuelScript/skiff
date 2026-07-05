package cmd

import (
	"fmt"

	"github.com/nuelScript/skiff/internal/panel"
	"github.com/spf13/cobra"
)

// newSelfUpdateCmd rebuilds Skiff from its own repo and hot-swaps the running
// control plane behind the router. It's launched internally by the webhook, not
// meant to be run by hand, so it's hidden.
func newSelfUpdateCmd() *cobra.Command {
	var repo, branch, commit, deployID string
	cmd := &cobra.Command{
		Use:    "self-update",
		Short:  "Rebuild and hot-swap the control plane from its own git repo",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}
			return panel.SelfUpdate(panel.SelfUpdateOpts{
				Repo: repo, Branch: branch, Commit: commit, DeployID: deployID,
			})
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "owner/name of Skiff's own repository")
	cmd.Flags().StringVar(&branch, "branch", "main", "branch to build")
	cmd.Flags().StringVar(&commit, "commit", "", "commit sha (recorded in history)")
	cmd.Flags().StringVar(&deployID, "deploy-id", "", "deploy id (history + log path)")
	return cmd
}
