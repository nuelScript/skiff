package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newGitSetupCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "git-setup",
		Short: "Set up `git push` deploys for this app",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				ui.Fail("Couldn't load config")
				return err
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			repo := filepath.Join(home, ".skiff", "git", cfg.Name+".git")
			work := filepath.Join(home, ".skiff", "checkouts", cfg.Name)

			ui.Banner(version)

			if _, err := os.Stat(repo); os.IsNotExist(err) {
				if out, err := exec.Command("git", "init", "--bare", repo).CombinedOutput(); err != nil {
					ui.Fail("git init failed")
					return fmt.Errorf("%s", strings.TrimSpace(string(out)))
				}
			}

			skiffPath, err := os.Executable()
			if err != nil {
				skiffPath = "skiff"
			}

			// post-receive: check out the pushed branch and deploy it — the default branch as the app, others as a <branch>-<app> preview.
			hook := "#!/bin/sh\nset -e\n" +
				"WORK=\"" + work + "\"\n" +
				"SKIFF=\"" + skiffPath + "\"\n" +
				"APP=\"" + cfg.Name + "\"\n" +
				"while read oldrev newrev ref; do\n" +
				"  branch=${ref#refs/heads/}\n" +
				"  slug=$(printf '%s' \"$branch\" | tr '/' '-')\n" +
				"  rm -rf \"$WORK\"\n" +
				"  mkdir -p \"$WORK\"\n" +
				"  git --work-tree=\"$WORK\" checkout -f \"$branch\"\n" +
				"  cd \"$WORK\"\n" +
				"  if [ \"$branch\" = main ] || [ \"$branch\" = master ]; then\n" +
				"    echo \"--- skiff: deploying $APP ($branch) ---\"\n" +
				"    \"$SKIFF\" deploy\n" +
				"  else\n" +
				"    echo \"--- skiff: preview $slug-$APP ($branch) ---\"\n" +
				"    \"$SKIFF\" deploy --name \"$slug-$APP\"\n" +
				"  fi\n" +
				"done\n"
			if err := os.WriteFile(filepath.Join(repo, "hooks", "post-receive"), []byte(hook), 0o755); err != nil {
				return err
			}

			ui.Done("git-push deploy ready for " + cfg.Name)
			ui.Field("repo", repo)
			ui.Note("add the remote and push to deploy:")
			fmt.Println("    git remote add skiff " + repo)
			fmt.Println("    git push skiff HEAD:main")
			fmt.Println()
			return nil
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", config.DefaultFile, "path to skiff.toml")
	return cmd
}
