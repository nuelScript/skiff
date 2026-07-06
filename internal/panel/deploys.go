package panel

import (
	"bufio"
	"context"
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
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/github"
)

// inflight tracks the currently-building deploy per app so a newer deploy can
// supersede (cancel) an older one still building, rather than racing it.
type inflightBuild struct {
	id     string
	cancel context.CancelFunc
}

var (
	inflightMu sync.Mutex
	inflight   = map[string]inflightBuild{}
)

// cancelInflight cancels the app's in-flight build if one is running (matching
// id when given). The build's own runDeploy then records it as "canceled".
func cancelInflight(app, id string) bool {
	inflightMu.Lock()
	defer inflightMu.Unlock()
	if b, ok := inflight[app]; ok && (id == "" || b.id == id) {
		b.cancel()
		return true
	}
	return false
}

// runDeploy clones a source, builds, and releases it, writing a persisted log
// and recording the deploy in history. Shared by manual deploys and webhooks.
// authURL, when set, overrides the clone URL (e.g. a pasted token); otherwise a
// GitHub-App token is used for repo sources.
func (p *Panel) runDeploy(src Source, authURL, commit, message, trigger, id string) {
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
		ID: id, App: src.App, Commit: shortCommit(commit), Message: message,
		Trigger: trigger, Status: "building", Started: time.Now().Unix(),
	})

	// Cancel any in-flight build of the same app, and register this one as the
	// current build so a later deploy can in turn supersede it.
	ctx, cancel := context.WithCancel(context.Background())
	inflightMu.Lock()
	if prev, ok := inflight[src.App]; ok {
		prev.cancel()
	}
	inflight[src.App] = inflightBuild{id: id, cancel: cancel}
	inflightMu.Unlock()
	defer func() {
		inflightMu.Lock()
		if b, ok := inflight[src.App]; ok && b.id == id {
			delete(inflight, src.App)
		}
		inflightMu.Unlock()
		cancel()
	}()
	// finish records the terminal status, mapping a cancellation (superseded by a
	// newer deploy) to "canceled" rather than "failed".
	finish := func(failMsg string) {
		if ctx.Err() != nil {
			logln("✗ superseded by a newer deploy")
			setDeployStatus(src.App, id, "canceled")
			return
		}
		logln(failMsg)
		setDeployStatus(src.App, id, "failed")
		go dispatchAlert(alertEvent{
			Team: src.Team, Kind: "deploy.failed", App: src.App,
			Title:  "Deploy failed: " + src.App,
			Detail: strings.TrimPrefix(strings.TrimSpace(failMsg), "✗ "),
		})
	}

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
	cl := exec.CommandContext(ctx, "git", args...)
	cl.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, e := cl.CombinedOutput(); e != nil {
		finish("✗ clone failed: " + cloneError(out))
		return
	}

	// Build from the project's root directory (monorepo support), safely joined
	// so a "../.." can't escape the clone.
	ctxDir := filepath.Join(work, filepath.Clean("/"+src.RootDir))
	if fi, err := os.Stat(ctxDir); err != nil || !fi.IsDir() {
		finish("✗ root directory not found in the repo: " + orRoot(src.RootDir))
		return
	}
	tomlPath := filepath.Join(ctxDir, "skiff.toml")
	_ = os.WriteFile(tomlPath, []byte(projectToml(src, deployEnv(src))), 0o644)

	logln("→ building & deploying")
	self, _ := os.Executable()
	cmd := exec.CommandContext(ctx, self, "deploy", "-c", tomlPath)
	// On supersede, interrupt (SIGINT) so the build cancels gracefully rather than
	// being hard-killed; fall back to a kill if it doesn't exit promptly.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 10 * time.Second
	cmd.Env = append(os.Environ(), "SKIFF_DEPLOY_ID="+id) // retain this build for rollback
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
		finish("✗ deploy failed")
		return
	}
	logln("✓ live")
	setDeployStatus(src.App, id, "live")
}

// runRollback re-runs a retained build image (skiff-<app>:<targetID>) with no
// rebuild, records it as a new deploy, and writes a persisted log streamed over
// SSE. commit/message are copied from the target so history reads sensibly.
func (p *Panel) runRollback(src Source, targetID, commit, message, id string) {
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
		ID: id, App: src.App, Commit: commit, Message: message,
		Trigger: "rollback", Status: "building", Started: time.Now().Unix(),
	})

	// A throwaway config dir carrying the current port/env/secrets for the run.
	work := filepath.Join(p.buildsDir, sanitizeName(src.App)+"-rollback")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		logln("✗ rollback failed: " + err.Error())
		setDeployStatus(src.App, id, "failed")
		return
	}
	tomlPath := filepath.Join(work, "skiff.toml")
	_ = os.WriteFile(tomlPath, []byte(projectToml(src, deployEnv(src))), 0o644)

	image := fmt.Sprintf("skiff-%s:%s", src.App, targetID)
	logln("→ rolling back to " + image)
	self, _ := os.Executable()
	cmd := exec.Command(self, "rollback", "--image", image, "-c", tomlPath)
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
		logln("✗ rollback failed")
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
	if app == "" {
		_ = json.NewEncoder(w).Encode(allDeploys())
		return
	}
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
	fmt.Fprintf(&b, "name = %q\n", src.App)
	if src.Replicas > 1 {
		fmt.Fprintf(&b, "replicas = %d\n", src.Replicas)
	}
	fmt.Fprintf(&b, "\n[build]\nport = %s\n", src.Port)
	b.WriteString("\n[deploy]\n")
	fmt.Fprintf(&b, "network = %q\n", teamNetwork(src.Team))
	if strings.TrimSpace(src.Release) != "" {
		fmt.Fprintf(&b, "release = %q\n", src.Release)
	}
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
