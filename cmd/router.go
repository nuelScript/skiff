package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/router"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newRouterCmd() *cobra.Command {
	var domain, httpAddr, panel, panelPointer, siteApp string
	cmd := &cobra.Command{
		Use:   "router",
		Short: "Run the edge router (subdomain routing + auto HTTPS) — runs on the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if domain == "" {
				return fmt.Errorf("--domain is required (e.g. --domain useskiff.xyz)")
			}
			if panelPointer == "" {
				home, _ := os.UserHomeDir()
				panelPointer = filepath.Join(home, ".skiff", "panel.addr")
			}
			rt := &router.Router{
				Domains:      strings.Split(domain, ","),
				Engine:       docker.Local(),
				Panel:        panel,
				PanelPointer: panelPointer,
				SiteApp:      siteApp,
			}

			ui.Banner(version)
			ui.Field("router", domain)
			if panel != "" {
				ui.Field("dashboard", "dash.* → "+panel)
			}
			if siteApp != "" {
				ui.Field("site", "apex + www → "+siteApp)
			}
			ui.Field("status", "status.* → live status page")
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
	cmd.Flags().StringVar(&panel, "panel", "127.0.0.1:7070", "fallback control panel address for dash.<domain>")
	cmd.Flags().StringVar(&panelPointer, "panel-pointer", "", "file holding the live panel address (default ~/.skiff/panel.addr)")
	cmd.Flags().StringVar(&siteApp, "site-app", "www", "app that serves the apex + www.<domain>")
	return cmd
}
