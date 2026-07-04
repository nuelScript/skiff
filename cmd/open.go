package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/proxy"
	"github.com/nuelScript/skiff/internal/registry"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newOpenCmd() *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "open <app>",
		Short: "Open a deployed app's URL in the browser",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			apps, err := registry.Load()
			if err != nil {
				return err
			}
			app, ok := apps[name]
			if !ok {
				ui.Fail("No app named " + name + " — run `skiff ls`")
				return fmt.Errorf("unknown app %q", name)
			}

			url := proxy.URL(app.Name)
			if app.Host != "" {
				url = fmt.Sprintf("http://%s:%d", docker.SSHHostname(app.Host), app.HostPort)
			}

			if printOnly {
				fmt.Println(url)
				return nil
			}
			if err := openURL(url); err != nil {
				ui.Fail("Couldn't open the browser")
				ui.Note(url)
				return err
			}
			ui.Done("Opened " + url)
			return nil
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the URL instead of opening a browser")
	return cmd
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
