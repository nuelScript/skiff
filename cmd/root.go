package cmd

import (
	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X .../cmd.version=<tag>";
// the default is the fallback for `go install`/dev builds.
var version = "0.1.0"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "skiff",
		Short: "Effortless deploys on your own server",
		Long: `Skiff deploys your apps to your own server with a single command:
build it, run it, and get an HTTPS URL. No cloud bill.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newInitCmd())
	root.AddCommand(newDeployCmd())
	root.AddCommand(newRollbackCmd())
	root.AddCommand(newGitSetupCmd())
	root.AddCommand(newServerCmd())
	root.AddCommand(newRouterCmd())
	root.AddCommand(newPanelCmd())
	root.AddCommand(newSelfUpdateCmd())
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
