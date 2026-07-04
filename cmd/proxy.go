package cmd

import (
	"fmt"

	"github.com/nuelScript/skiff/internal/proxy"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newProxyCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Run the local router for *.localhost apps",
		RunE: func(cmd *cobra.Command, args []string) error {
			ui.Banner(version)
			ui.Field("router", addr)
			ui.Note("routing *.localhost → your deployed apps  (ctrl-c to stop)")
			fmt.Println()
			return proxy.Serve(addr)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", proxy.DefaultAddr, "address to listen on")
	return cmd
}
