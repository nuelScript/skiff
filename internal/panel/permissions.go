package panel

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nuelScript/skiff/internal/config"
)

// handlePermissions shows an app's machine permissions, and lets the server's admin grant or remove them. They reach the containers on the next deploy.
func (p *Panel) handlePermissions(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{
			"granted":  config.DecodePermissions(src.Permissions),
			"wants":    config.DecodePermissions(src.WantsPermissions),
			"canGrant": p.isInstanceAdmin(r),
			"strong":   config.StrongCapabilities,
		})
	case http.MethodPut:
		if !p.isInstanceAdmin(r) {
			http.Error(w, "only the server's admin, an owner of the team that set it up, can grant machine permissions", http.StatusForbidden)
			return
		}
		if src.Parent != "" {
			http.Error(w, "a preview takes its project's permissions", http.StatusBadRequest)
			return
		}
		var body config.Permissions
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body = body.Normalize()
		if err := body.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		src.Permissions = config.EncodePermissions(body)
		if err := putSource(src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.audit(r, "project.permissions", app, describePermissions(body))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// describePermissions is the Activity line for a grant.
func describePermissions(g config.Permissions) string {
	if g.IsZero() {
		return "none"
	}
	var parts []string
	if g.Privileged {
		parts = append(parts, "privileged")
	}
	if g.HostNetwork {
		parts = append(parts, "host network")
	}
	if g.NewPrivileges {
		parts = append(parts, "setuid")
	}
	if len(g.Capabilities) > 0 {
		parts = append(parts, "capabilities "+strings.Join(g.Capabilities, ","))
	}
	if len(g.Devices) > 0 {
		parts = append(parts, "devices "+strings.Join(g.Devices, ","))
	}
	if len(g.Sysctls) > 0 {
		parts = append(parts, "sysctls "+strings.Join(g.Sysctls, ","))
	}
	return strings.Join(parts, "; ")
}
