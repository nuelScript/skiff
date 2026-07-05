package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"

	"github.com/coder/websocket"
	"github.com/creack/pty"

	"github.com/nuelScript/skiff/internal/registry"
)

// handleExec opens an interactive shell inside an app's container over a
// WebSocket — a browser terminal for running migrations, inspecting state, etc.
// A PTY is used so docker sees a real TTY (line editing, colours, curses apps).
func (p *Panel) handleExec(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	if !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	apps, err := registry.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a, ok := apps[app]
	if !ok {
		http.Error(w, "unknown app", http.StatusNotFound)
		return
	}
	if a.Host != "" {
		http.Error(w, "console isn't available for remote apps yet", http.StatusBadRequest)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()

	// r.Context() is canceled once Accept hijacks the connection, so the session
	// gets its own context, canceled when either side of the bridge goes away.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Prefer bash, fall back to sh — whichever the image ships. Check bash exists
	// before exec-ing it, since a failed exec would kill the shell outright.
	cmd := exec.CommandContext(ctx, "docker", "exec", "-it", a.Container,
		"sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		_ = c.Write(ctx, websocket.MessageText, []byte("\r\n\x1b[31mcouldn't open a shell in this container\x1b[0m\r\n"))
		return
	}
	defer func() { _ = ptmx.Close() }()

	// Container output → browser.
	go func() {
		buf := make([]byte, 8192)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				if werr := c.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					cancel()
					return
				}
			}
			if rerr != nil {
				cancel()
				return
			}
		}
	}()

	// Browser input (keystrokes + resize) → container.
	var msg struct {
		T string `json:"t"`
		D string `json:"d"`
		C int    `json:"c"`
		R int    `json:"r"`
	}
	for {
		_, data, rerr := c.Read(ctx)
		if rerr != nil {
			return
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.T {
		case "in":
			_, _ = ptmx.WriteString(msg.D)
		case "resize":
			if msg.C > 0 && msg.R > 0 {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(msg.R), Cols: uint16(msg.C)})
			}
		}
	}
}
