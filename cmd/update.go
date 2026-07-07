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

// updateInfo is the on-disk cache of the last-seen latest release.
type updateInfo struct {
	CheckedAt int64  `json:"checkedAt"`
	Latest    string `json:"latest"` // e.g. "0.1.1"
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

// notifyUpdate prints a one-line hint when a newer release exists, and refreshes
// the cached latest version — in a detached process — at most once a day. The
// hot path only reads a local cache, so it never blocks the command; it stays
// silent on any error, for dev builds, in CI (SKIFF_NO_UPDATE_CHECK), and when
// stderr isn't a terminal.
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

// updateNotice returns the one-line hint to show when latest is newer than
// current, or "" when there's nothing to say.
func updateNotice(current, latest string) string {
	if latest == "" || !versionLess(current, latest) {
		return ""
	}
	return fmt.Sprintf("\n  ↑ skiff %s is available (you have %s). Update:  %s\n",
		latest, current, updateInstallCmd)
}

// refreshUpdateCacheDetached re-invokes skiff to fetch and cache the latest
// version without blocking: it starts the child and never waits, so the check
// happens in the background (and outlives this command) with no output.
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

// newUpdateCheckCmd is the hidden command the detached refresh runs: it fetches
// the latest release and writes the cache, then exits.
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

// versionLess reports whether a < b for simple dotted versions; any pre-release
// suffix after "-" is ignored.
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
