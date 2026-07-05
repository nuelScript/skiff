package panel

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nuelScript/skiff/internal/github"
)

// runDeploy clones a source, builds, and releases it, writing a persisted log
// and recording the deploy in history. Shared by manual deploys and webhooks.
// authURL, when set, overrides the clone URL (e.g. a pasted token); otherwise a
// GitHub-App token is used for repo sources.
func (p *Panel) runDeploy(src Source, authURL, commit, trigger, id string) {
	logp := logPath(src.App, id)
	if err := os.MkdirAll(filepath.Dir(logp), 0o755); err != nil {
		return
	}
	f, err := os.Create(logp)
	if err != nil {
		return
	}
	defer f.Close()
	logln := func(s string) { fmt.Fprintln(f, s) }

	addDeploy(Deploy{
		ID: id, App: src.App, Commit: shortCommit(commit),
		Trigger: trigger, Status: "building", Started: time.Now().Unix(),
	})

	clone := authURL
	if clone == "" {
		clone = src.CloneURL
		if src.Repo != "" {
			if gh := github.Load(); gh.Installed() {
				if u, e := gh.CloneURLWithToken(src.CloneURL); e == nil {
					clone = u
				}
			}
		}
	}

	work := filepath.Join(p.buildsDir, sanitizeName(src.App))
	_ = os.RemoveAll(work)

	label := src.Repo
	if label == "" {
		label = src.CloneURL
	}
	logln("→ cloning " + label + "  (" + orMain(src.Branch) + ")")
	args := []string{"clone", "--depth", "1"}
	if src.Branch != "" {
		args = append(args, "--branch", src.Branch)
	}
	args = append(args, clone, work)
	cl := exec.Command("git", args...)
	cl.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, e := cl.CombinedOutput(); e != nil {
		logln("✗ clone failed: " + cloneError(out))
		setDeployStatus(src.App, id, "failed")
		return
	}

	// Build from the project's root directory (monorepo support), safely joined
	// so a "../.." can't escape the clone.
	ctxDir := filepath.Join(work, filepath.Clean("/"+src.RootDir))
	if fi, err := os.Stat(ctxDir); err != nil || !fi.IsDir() {
		logln("✗ root directory not found in the repo: " + orRoot(src.RootDir))
		setDeployStatus(src.App, id, "failed")
		return
	}
	tomlPath := filepath.Join(ctxDir, "skiff.toml")
	_ = os.WriteFile(tomlPath, []byte(projectToml(src, getEnv(src.App))), 0o644)

	logln("→ building & deploying")
	self, _ := os.Executable()
	cmd := exec.Command(self, "deploy", "-c", tomlPath)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	errc := make(chan error, 1)
	go func() { errc <- cmd.Run(); _ = pw.Close() }()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		logln(sc.Text())
	}
	if e := <-errc; e != nil {
		logln("✗ deploy failed")
		setDeployStatus(src.App, id, "failed")
		return
	}
	logln("✓ live")
	setDeployStatus(src.App, id, "live")
}

// tailLog streams a deploy's persisted log over SSE, following it live until the
// deploy reaches a terminal state.
func (p *Panel) tailLog(w http.ResponseWriter, r *http.Request, app, id string) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	logp := logPath(app, id)
	sent := 0
	flush := func() {
		for _, ln := range readLogLines(logp)[sent:] {
			fmt.Fprintf(w, "data: %s\n\n", ln)
			sent++
		}
		fl.Flush()
	}
	for {
		flush()
		switch deployStatus(app, id) {
		case "live":
			flush()
			fmt.Fprint(w, "data: [done] ok\n\n")
			fl.Flush()
			return
		case "failed":
			flush()
			fmt.Fprint(w, "data: [done] error\n\n")
			fl.Flush()
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(400 * time.Millisecond):
		}
	}
}

func (p *Panel) handleDeployLog(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	id := sanitizeID(r.URL.Query().Get("id"))
	p.tailLog(w, r, app, id)
}

func (p *Panel) handleDeploys(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(appDeploys(app))
}

func readLogLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func newDeployID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d-%s", time.Now().Unix(), hex.EncodeToString(b))
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

func orMain(b string) string {
	if b == "" {
		return "default branch"
	}
	return b
}

func orRoot(d string) string {
	if d == "" {
		return "/"
	}
	return d
}

// projectToml renders the skiff.toml the panel deploys with: name, port, and the
// project's env vars split into [env] (build+runtime) and [secrets] (runtime).
func projectToml(src Source, env []EnvVar) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name = %q\n\n[build]\nport = %s\n", src.App, src.Port)
	var buildVars, secretVars []EnvVar
	for _, e := range env {
		if e.Build {
			buildVars = append(buildVars, e)
		} else {
			secretVars = append(secretVars, e)
		}
	}
	if len(buildVars) > 0 {
		b.WriteString("\n[env]\n")
		for _, e := range buildVars {
			fmt.Fprintf(&b, "%s = %q\n", e.Key, e.Value)
		}
	}
	if len(secretVars) > 0 {
		b.WriteString("\n[secrets]\n")
		for _, e := range secretVars {
			fmt.Fprintf(&b, "%s = %q\n", e.Key, e.Value)
		}
	}
	return b.String()
}
