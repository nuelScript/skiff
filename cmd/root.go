package cmd

import (
	"github.com/spf13/cobra"
)

const version = "0.1.0"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "skiff",
		Short: "Effortless deploys on your own server",
		Long: `Skiff deploys your apps to your own server with a single command:
build it, run it, and get an HTTPS URL. No cloud bill.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newInitCmd())
	root.AddCommand(newDeployCmd())
	root.AddCommand(newGitSetupCmd())
	root.AddCommand(newProxyCmd())
	root.AddCommand(newDashboardCmd())
	root.AddCommand(newLsCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newOpenCmd())
	root.AddCommand(newLogsCmd())
	root.AddCommand(newDownCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newVersionCmd())
	return root
}

func Execute() error {
	return newRootCmd().Execute()
}
