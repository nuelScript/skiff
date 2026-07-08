package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	updateRepo       = "nuelScript/skiff"
	updateInterval   = 24 * time.Hour
	updateCheckCmd   = "__update-check"
	updateInstallCmd = "curl -fsSL https://useskiff.xyz/cli | sh"
)

type updateInfo struct {
	CheckedAt int64  `json:"checkedAt"`
	Latest    string `json:"latest"`
}

func updateCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".skiff", "update-check.json")
}

func readUpdateCache() updateInfo {
	var u updateInfo
	if p := updateCachePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &u)
		}
	}
	return u
}

func writeUpdateCache(u updateInfo) {
	p := updateCachePath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if b, err := json.Marshal(u); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// notifyUpdate hints when a newer release exists and refreshes the cache in a detached process at most daily, so the hot path only reads a local cache and never blocks.
func notifyUpdate(current string) {
	if current == "" || current == "dev" || os.Getenv("SKIFF_NO_UPDATE_CHECK") != "" {
		return
	}
	if !isTerminal(os.Stderr) {
		return
	}
	cache := readUpdateCache()
	if msg := updateNotice(current, cache.Latest); msg != "" {
		fmt.Fprint(os.Stderr, msg)
	}
	if time.Now().Unix()-cache.CheckedAt > int64(updateInterval.Seconds()) {
		refreshUpdateCacheDetached()
	}
}

func updateNotice(current, latest string) string {
	if latest == "" || !versionLess(current, latest) {
		return ""
	}
	return fmt.Sprintf("\n  ↑ skiff %s is available (you have %s). Update:  %s\n",
		latest, current, updateInstallCmd)
}

// refreshUpdateCacheDetached re-invokes skiff to refresh the version cache in a detached child that outlives this command.
func refreshUpdateCacheDetached() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	c := exec.Command(exe, updateCheckCmd)
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	_ = c.Start() // fire and forget — do not Wait
}

func fetchLatestVersion() (string, bool) {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet,
		"https://api.github.com/repos/"+updateRepo+"/releases/latest", nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var out struct {
		TagName string `json:"tag_name"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || out.TagName == "" {
		return "", false
	}
	return strings.TrimPrefix(out.TagName, "v"), true
}

// newUpdateCheckCmd is the hidden command the detached refresh runs to fetch the latest release and write the cache.
func newUpdateCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:    updateCheckCmd,
		Hidden: true,
		Run: func(_ *cobra.Command, _ []string) {
			if latest, ok := fetchLatestVersion(); ok {
				writeUpdateCache(updateInfo{CheckedAt: time.Now().Unix(), Latest: latest})
			}
		},
	}
}

// versionLess reports whether a < b for dotted versions; any "-" pre-release suffix is ignored.
func versionLess(a, b string) bool {
	pa, pb := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	s = strings.SplitN(strings.TrimPrefix(s, "v"), "-", 2)[0]
	var out [3]int
	for i, p := range strings.Split(s, ".") {
		if i > 2 {
			break
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
