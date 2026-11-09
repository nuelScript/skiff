package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/docker"
)

// A sandbox is a throwaway container an agent asks for to run code it wrote: no network unless asked, no privileges, removed when the command ends.
const (
	sandboxDefaultImage   = "python:3.12-alpine"
	sandboxDefaultTimeout = 2 * time.Minute
	sandboxMaxTimeout     = 15 * time.Minute
	sandboxMemory         = "512m"
	sandboxCPU            = "1"
	sandboxPerApp         = 3
	sandboxKeep           = 50
	sandboxMaxRequest     = 32 << 20
)

var sandboxes = struct {
	sync.Mutex
	next int64
	live map[string]map[int64]context.CancelFunc // by app
}{live: map[string]map[int64]context.CancelFunc{}}

type sandboxRequest struct {
	Image       string            `json:"image"`
	Command     string            `json:"command"`
	Files       map[string]string `json:"files"`       // text
	BinaryFiles map[string]string `json:"binaryFiles"` // base64
	Internet    bool              `json:"internet"`
	Timeout     int               `json:"timeout"` // seconds
}

type sandboxResponse struct {
	ID        int64             `json:"id"`
	ExitCode  int               `json:"exitCode"`
	Output    string            `json:"output"`
	Truncated bool              `json:"truncated,omitempty"`
	Files     map[string]string `json:"files"` // base64
	Skipped   []string          `json:"skipped,omitempty"`
	Seconds   float64           `json:"seconds"`
	TimedOut  bool              `json:"timedOut,omitempty"`
}

// sandboxSpec checks a request and turns it into what Docker runs.
func sandboxSpec(app string, req sandboxRequest) (docker.SandboxSpec, time.Duration, error) {
	spec := docker.SandboxSpec{App: app, Image: req.Image, Command: req.Command, Internet: req.Internet,
		Memory: sandboxMemory, CPU: sandboxCPU, Files: map[string][]byte{}}
	if spec.Image == "" {
		spec.Image = sandboxDefaultImage
	}
	if err := config.ValidateImage(spec.Image); err != nil {
		return spec, 0, err
	}
	if spec.Command == "" {
		return spec, 0, errors.New("give a command to run, like python main.py")
	}
	for p, text := range req.Files {
		clean, ok := docker.SandboxPath(p)
		if !ok {
			return spec, 0, fmt.Errorf("%q isn't a path inside the sandbox; use one like src/main.py", p)
		}
		spec.Files[clean] = []byte(text)
	}
	for p, b64 := range req.BinaryFiles {
		clean, ok := docker.SandboxPath(p)
		if !ok {
			return spec, 0, fmt.Errorf("%q isn't a path inside the sandbox; use one like data/input.bin", p)
		}
		b, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return spec, 0, fmt.Errorf("%s isn't base64", p)
		}
		spec.Files[clean] = b
	}
	timeout := sandboxDefaultTimeout
	if req.Timeout > 0 {
		timeout = min(time.Duration(req.Timeout)*time.Second, sandboxMaxTimeout)
	}
	return spec, timeout, nil
}

// claimSandbox holds one of the app's sandbox places until release is called.
func claimSandbox(app string, cancel context.CancelFunc) (release func(), ok bool) {
	sandboxes.Lock()
	defer sandboxes.Unlock()
	if len(sandboxes.live[app]) >= sandboxPerApp {
		return nil, false
	}
	if sandboxes.live[app] == nil {
		sandboxes.live[app] = map[int64]context.CancelFunc{}
	}
	sandboxes.next++
	key := sandboxes.next
	sandboxes.live[app][key] = cancel
	return func() {
		sandboxes.Lock()
		defer sandboxes.Unlock()
		delete(sandboxes.live[app], key)
	}, true
}

// stopSandboxes ends every sandbox an app has running.
func (p *Panel) stopSandboxes(app string) {
	sandboxes.Lock()
	for _, cancel := range sandboxes.live[app] {
		cancel()
	}
	sandboxes.Unlock()
	p.eng.RemoveSandboxes(app)
}

func (p *Panel) runSandbox(w http.ResponseWriter, r *http.Request, a aiApp) {
	var req sandboxRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, sandboxMaxRequest)).Decode(&req) != nil {
		gatewayError(w, http.StatusBadRequest, "send JSON with a command, and files as {path: text}")
		return
	}
	spec, timeout, err := sandboxSpec(a.App, req)
	if err != nil {
		gatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	release, ok := claimSandbox(a.App, cancel)
	if !ok {
		gatewayError(w, http.StatusTooManyRequests, fmt.Sprintf("this agent already has %d sandboxes running; wait for one to finish", sandboxPerApp))
		return
	}
	defer release()
	// The caller hanging up ends the sandbox too.
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()

	started := time.Now()
	id := recordSandboxStart(a.App, spec, started)
	res, err := p.eng.Sandbox(ctx, spec)
	out := sandboxResponse{ID: id, ExitCode: res.ExitCode, Output: res.Output, Truncated: res.Truncated,
		Files: map[string]string{}, Skipped: res.Skipped, Seconds: time.Since(started).Seconds()}
	for path, b := range res.Files {
		out.Files[path] = base64.StdEncoding.EncodeToString(b)
	}
	status := "ok"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		out.TimedOut, out.ExitCode, status = true, -1, "timeout"
	case errors.Is(err, context.Canceled):
		out.ExitCode, status = -1, "stopped"
	case err != nil:
		recordSandboxEnd(id, "failed", -1, err.Error())
		gatewayError(w, http.StatusBadGateway, err.Error())
		return
	case res.ExitCode != 0:
		status = "failed"
	}
	recordSandboxEnd(id, status, out.ExitCode, res.Output)
	writeJSON(w, out)
}

func recordSandboxStart(app string, spec docker.SandboxSpec, now time.Time) int64 {
	res, err := sqlDB.Exec(`INSERT INTO sandboxes(app, image, command, internet, files, started, status) VALUES(?,?,?,?,?,?,'running')`,
		app, spec.Image, spec.Command, b2i(spec.Internet), len(spec.Files), now.Unix())
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	_, _ = sqlDB.Exec(`DELETE FROM sandboxes WHERE app=? AND id NOT IN (SELECT id FROM sandboxes WHERE app=? ORDER BY id DESC LIMIT ?)`, app, app, sandboxKeep)
	return id
}

func recordSandboxEnd(id int64, status string, code int, output string) {
	if len(output) > 4096 {
		output = output[len(output)-4096:]
	}
	_, _ = sqlDB.Exec(`UPDATE sandboxes SET finished=?, status=?, exit_code=?, output=? WHERE id=?`, time.Now().Unix(), status, code, output, id)
}

type sandboxRow struct {
	ID       int64  `json:"id"`
	Image    string `json:"image"`
	Command  string `json:"command"`
	Internet bool   `json:"internet"`
	Files    int    `json:"files"`
	Started  int64  `json:"started"`
	Finished int64  `json:"finished"`
	Status   string `json:"status"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
}

func recentSandboxes(app string, limit int) []sandboxRow {
	rows, err := sqlDB.Query(`SELECT id, image, command, internet, files, started, finished, status, exit_code, output FROM sandboxes WHERE app=? ORDER BY id DESC LIMIT ?`, app, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []sandboxRow{}
	for rows.Next() {
		var s sandboxRow
		var internet int
		if rows.Scan(&s.ID, &s.Image, &s.Command, &internet, &s.Files, &s.Started, &s.Finished, &s.Status, &s.ExitCode, &s.Output) == nil {
			s.Internet = internet != 0
			out = append(out, s)
		}
	}
	return out
}

// clearLeftoverSandboxes removes sandboxes a previous panel was running when it stopped; nobody is waiting for them now.
func (p *Panel) clearLeftoverSandboxes() {
	p.eng.RemoveSandboxes("")
	_, _ = sqlDB.Exec(`UPDATE sandboxes SET status='stopped', finished=? WHERE status='running'`, time.Now().Unix())
}
