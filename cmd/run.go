package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "run <app> -- <command>",
		Short: "Run a command once in a fresh container from an app's latest build",
		Long: `Run a command once in a fresh container from an app's latest build, with its environment and limits,
and print its output as it comes. Skiff exits with the command's exit code. Ctrl-C stops the run.

  skiff run shop-api -- npm run migrate
  skiff run shop-api --timeout 2h -- "python backfill.py && echo done"`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, command := args[0], strings.Join(args[1:], " ")
			c, err := remoteClient(time.Minute)
			if err != nil {
				return err
			}
			var run struct {
				ID       int64  `json:"id"`
				Status   string `json:"status"`
				ExitCode int    `json:"exitCode"`
			}
			base := "/apps/" + url.PathEscape(app) + "/runs"
			if err := c.do(http.MethodPost, base, map[string]any{"command": command, "timeout": int(timeout / time.Second)}, &run); err != nil {
				return err
			}
			path := base + "/" + strconv.FormatInt(run.ID, 10)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			go func() {
				<-ctx.Done()
				_ = c.do(http.MethodDelete, path, nil, nil)
			}()
			req, err := http.NewRequest(http.MethodGet, c.base+"/api/v1"+path+"/logs", nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+c.token)
			resp, err := (&http.Client{}).Do(req)
			if err != nil {
				return err
			}
			_, _ = io.Copy(os.Stdout, resp.Body)
			resp.Body.Close()

			if err := c.do(http.MethodGet, path, nil, &run); err != nil {
				return err
			}
			switch run.Status {
			case "ok":
				return nil
			case "timeout":
				ui.Fail("Stopped at its timeout")
			case "running":
				ui.Fail("Lost the connection while it was still running. It goes on on the server, at GET /api/v1" + path)
			default:
				ui.Fail(fmt.Sprintf("Exited with %d", run.ExitCode))
			}
			os.Exit(max(run.ExitCode, 1))
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "how long it may take, like 2h (default 30m, up to 168h)")
	return cmd
}
