package cmd

import (
	"fmt"
	"strings"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/router"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newRouterCmd() *cobra.Command {
	var domain, httpAddr string
	cmd := &cobra.Command{
		Use:   "router",
		Short: "Run the edge router (subdomain routing + auto HTTPS) — runs on the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if domain == "" {
				return fmt.Errorf("--domain is required (e.g. --domain useskiff.xyz)")
			}
			rt := &router.Router{Domains: strings.Split(domain, ","), Engine: docker.Local()}

			ui.Banner(version)
			ui.Field("router", domain)
			if httpAddr != "" {
				ui.Note("http-only test mode on " + httpAddr)
				fmt.Println()
				return rt.ServeHTTPOnly(httpAddr)
			}
			ui.Note("serving :80 (ACME) + :443 (auto HTTPS)")
			fmt.Println()
			return rt.ServeTLS("/var/lib/skiff/certs")
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "base domain — apps served at <app>.<domain>")
	cmd.Flags().StringVar(&httpAddr, "http-addr", "", "HTTP-only test mode on this address (no TLS)")
	return cmd
}
