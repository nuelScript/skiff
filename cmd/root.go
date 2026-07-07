package cmd

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// buildVersion is injected at release time via
// -ldflags "-X github.com/nuelScript/skiff/cmd.buildVersion=<tag>". Empty in
// every other build.
var buildVersion = ""

// version is the resolved, display-ready version everything prints.
var version = resolveVersion()

// resolveVersion prefers the ldflags-injected release tag, then the clean module
// version stamped into `go install …@vX` builds (that path gets no ldflags), and
// otherwise reports "dev" — with any leading "v" trimmed for consistent output.
func resolveVersion() string {
	if buildVersion != "" {
		return strings.TrimPrefix(buildVersion, "v")
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		// A tagged `go install` yields e.g. "v0.1.1"; a local/dirty build yields
		// "(devel)" or a pseudo-version with "+", which we treat as dev.
		if v := info.Main.Version; strings.HasPrefix(v, "v") && !strings.Contains(v, "+") {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}

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
