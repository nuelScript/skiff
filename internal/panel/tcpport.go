package panel

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// reservedTCPPorts are used by Skiff itself or by the server's own services.
var reservedTCPPorts = map[int]string{22: "SSH", 25: "email", 80: "the web", 443: "the web", 7070: "the dashboard", 7071: "the dashboard"}

// lowPortsNeed explains who can open a port under 1024: they're the well-known services' ports, so the server's admin decides.
const lowPortsNeed = "ports under 1024 can only be opened by the server's admin, an owner of the team that set it up"

func portRange(port int, admin bool) string {
	switch {
	case port < 1 || port > 65535:
		return "use a port from 1 to 65535"
	case port < 1024 && !admin:
		return lowPortsNeed
	}
	return ""
}

func portTaken(port int, holder string) string {
	if holder == "" {
		return fmt.Sprintf("port %d is in use on this server", port)
	}
	return fmt.Sprintf("port %d is already used by %s", port, holder)
}

// teamOnly names a holder only for its own team, so one team can't learn what another runs.
func teamOnly(holderTeam, team, name string) string {
	if holderTeam != team {
		return ""
	}
	return name
}

func tcpPortProblem(port int, app, team string, sources []Source, usedByDBs map[int]string, admin bool) string {
	if problem := portRange(port, admin); problem != "" {
		return problem
	}
	if why, ok := reservedTCPPorts[port]; ok {
		return fmt.Sprintf("port %d is used by %s", port, why)
	}
	for _, s := range sources {
		if s.TCPPort == port && s.App != app {
			return portTaken(port, teamOnly(s.Team, team, s.App))
		}
	}
	if name, ok := usedByDBs[port]; ok {
		if name != "" {
			name = "the database " + name
		}
		return portTaken(port, name)
	}
	return ""
}

// publicDatabasePorts maps each public database port to its name, left empty for other teams' databases.
func publicDatabasePorts(team string) map[int]string {
	out := map[int]string{}
	rows, err := sqlDB.Query(`SELECT public_port, name, team FROM databases WHERE public=1`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var port int
		var name, owner string
		if rows.Scan(&port, &name, &owner) == nil {
			out[port] = teamOnly(owner, team, name)
		}
	}
	return out
}

// handleTCPPort opens or closes a public port for a project. It reaches the internet directly, so only a team owner can change it.
func (p *Panel) handleTCPPort(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"port": src.TCPPort, "containerPort": src.Port, "lowPorts": p.isInstanceAdmin(r)})
	case http.MethodPut:
		if !p.isOwner(r) {
			http.Error(w, "only a team owner can open a public port", http.StatusForbidden)
			return
		}
		if src.Parent != "" {
			http.Error(w, "previews can't have a public TCP port", http.StatusBadRequest)
			return
		}
		var body struct {
			Port int `json:"port"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Port != 0 && body.Port != src.TCPPort {
			if problem := tcpPortProblem(body.Port, app, src.Team, allSources(), publicDatabasePorts(src.Team), p.isInstanceAdmin(r)); problem != "" {
				http.Error(w, problem, http.StatusBadRequest)
				return
			}
			// Something else on the server may hold it, and the router would only be able to log that.
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", body.Port))
			if err != nil {
				http.Error(w, fmt.Sprintf("port %d is in use on this server", body.Port), http.StatusBadRequest)
				return
			}
			_ = ln.Close()
		}
		src.TCPPort = body.Port
		if body.Port != 0 && src.HealthPath == "" {
			src.HealthMode = "tcp"
		} else if body.Port == 0 && src.HealthMode == "tcp" && src.Stack == "" {
			src.HealthMode = ""
		}
		if err := putSource(src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = writeEdgeFile()
		p.audit(r, "project.tcp-port", app, fmt.Sprint(body.Port))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func udpPortProblem(port int, app, team string, sources []Source, admin bool) string {
	if problem := portRange(port, admin); problem != "" {
		return problem
	}
	for _, s := range sources {
		if s.UDPPort == port && s.App != app {
			if holder := teamOnly(s.Team, team, s.App); holder != "" {
				return fmt.Sprintf("UDP port %d is already used by %s", port, holder)
			}
			return fmt.Sprintf("UDP port %d is in use on this server", port)
		}
	}
	return ""
}

// handleUDPPort opens or closes a public UDP port for a project. The app listens on the same number, and needs a redeploy to publish it.
func (p *Panel) handleUDPPort(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"port": src.UDPPort, "lowPorts": p.isInstanceAdmin(r)})
	case http.MethodPut:
		if !p.isOwner(r) {
			http.Error(w, "only a team owner can open a public port", http.StatusForbidden)
			return
		}
		if src.Parent != "" {
			http.Error(w, "previews can't have a public UDP port", http.StatusBadRequest)
			return
		}
		var body struct {
			Port int `json:"port"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Port != 0 && body.Port != src.UDPPort {
			if problem := udpPortProblem(body.Port, app, src.Team, allSources(), p.isInstanceAdmin(r)); problem != "" {
				http.Error(w, problem, http.StatusBadRequest)
				return
			}
			pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", body.Port))
			if err != nil {
				http.Error(w, fmt.Sprintf("UDP port %d is in use on this server", body.Port), http.StatusBadRequest)
				return
			}
			_ = pc.Close()
		}
		src.UDPPort = body.Port
		if err := putSource(src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = writeEdgeFile()
		p.audit(r, "project.udp-port", app, fmt.Sprint(body.Port))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
