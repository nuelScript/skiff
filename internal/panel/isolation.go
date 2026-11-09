package panel

import (
	"encoding/json"
	"net/http"

	"github.com/nuelScript/skiff/internal/docker"
)

func runtimeFor(isolation string) string {
	if isolation == "strong" {
		return docker.SandboxRuntime
	}
	return ""
}

// handleIsolation shows and changes how strongly an app is sandboxed. Strong isolation only takes away, so any team owner can choose it; it needs gVisor on the server.
func (p *Panel) handleIsolation(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"isolation": src.Isolation, "available": p.eng.HasRuntime(docker.SandboxRuntime)})
	case http.MethodPut:
		if !p.isOwner(r) {
			http.Error(w, "only a team owner can change an app's isolation", http.StatusForbidden)
			return
		}
		var body struct {
			Isolation string `json:"isolation"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body) != nil || body.Isolation != "" && body.Isolation != "strong" {
			http.Error(w, `isolation is "strong" or empty`, http.StatusBadRequest)
			return
		}
		if body.Isolation == "strong" && !p.eng.HasRuntime(docker.SandboxRuntime) {
			http.Error(w, "strong isolation needs gVisor installed on the server first", http.StatusBadRequest)
			return
		}
		src.Isolation = body.Isolation
		if err := putSource(src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.audit(r, "project.isolation", app, body.Isolation)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
