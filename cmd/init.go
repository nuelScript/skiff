package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nuelScript/skiff/internal/builder"
	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a skiff.toml for the app in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			ui.Banner(version)

			if _, err := os.Stat(config.DefaultFile); err == nil {
				ui.Fail(config.DefaultFile + " already exists")
				return fmt.Errorf("%s already exists", config.DefaultFile)
			}

			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			name := filepath.Base(dir)

			stack := "unknown — add a Dockerfile"
			if b, err := builder.Select(".", "Dockerfile"); err == nil {
				stack = b.Name()
			}
			port := guessPort(".")

			content := fmt.Sprintf("name = %q\n\n[build]\nport = %d\n", name, port)
			if err := os.WriteFile(config.DefaultFile, []byte(content), 0o644); err != nil {
				return err
			}

			ui.Done("Wrote " + config.DefaultFile)
			ui.Field("name", name)
			ui.Field("stack", stack)
			ui.Field("port", fmt.Sprintf("%d", port))
			ui.Note("Set the port to match what your app listens on, then run `skiff deploy`.")
			fmt.Println()
			return nil
		},
	}
}

// guessPort picks a sensible default port from the files in dir.
func guessPort(dir string) int {
	has := func(f string) bool {
		_, err := os.Stat(filepath.Join(dir, f))
		return err == nil
	}
	switch {
	case has("package.json"):
		return 3000
	case has("requirements.txt"), has("pyproject.toml"), has("app.py"), has("main.py"):
		return 8000
	default:
		return 8080
	}
}
