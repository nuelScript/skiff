package panel

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Domain is a custom hostname pointed at one of the team's apps. The router
// serves it (with an on-demand Let's Encrypt cert) once DNS points at the box.
type Domain struct {
	Host       string   `json:"host"`
	App        string   `json:"app"`
	Created    int64    `json:"created"`
	PointsHere bool     `json:"pointsHere"`           // computed: DNS resolves to this server
	ResolvesTo []string `json:"resolvesTo,omitempty"` // computed: where the host currently points
}

// domainsResponse carries the team's domains plus this server's public IP, so
// the dashboard can show the exact A record to set.
type domainsResponse struct {
	ServerIP string   `json:"serverIp"`
	Domains  []Domain `json:"domains"`
}

var hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// normalizeHost cleans user input to a bare hostname and rejects anything that
// isn't a valid custom domain — including subdomains of the base domain, which
// the router already serves automatically.
func normalizeHost(host, served string) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexAny(h, "/:"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	if h == "" || len(h) > 253 || !hostRe.MatchString(h) {
		return "", false
	}
	if h == served || strings.HasSuffix(h, "."+served) {
		return "", false // already served at <sub>.<domain>
	}
	return h, true
}

func domainsFilePath() string { return filepath.Join(skiffDir(), "domains.json") }

// writeDomainsFile mirrors the host→app map to a file the edge router reads
// (cached briefly) for routing + its ACME host policy. Called after any change.
func writeDomainsFile() {
	rows, err := sqlDB.Query(`SELECT host, app FROM domains`)
	if err != nil {
		return
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var host, app string
		if rows.Scan(&host, &app) == nil {
			m[host] = app
		}
	}
	if b, err := json.Marshal(m); err == nil {
		_ = os.WriteFile(domainsFilePath(), b, 0o644)
	}
}

func teamDomains(team string) []Domain {
	rows, err := sqlDB.Query(
		`SELECT host, app, created FROM domains WHERE team=? ORDER BY app, host`, team)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		var d Domain
		if rows.Scan(&d.Host, &d.App, &d.Created) == nil {
			out = append(out, d)
		}
	}
	return out
}

func domainOwner(host string) (Domain, bool) {
	var d Domain
	err := sqlDB.QueryRow(`SELECT host, app FROM domains WHERE host=?`, host).Scan(&d.Host, &d.App)
	return d, err == nil
}

// handleDomains manages a team's custom domains: list (GET), add (POST), remove
// (DELETE). Each is scoped to apps the caller can access.
func (p *Panel) handleDomains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		domains := teamDomains(p.teamID(r))
		ip := serverPublicIP(p.domain)
		resolveDomains(domains, ip)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domainsResponse{ServerIP: ip, Domains: domains})

	case http.MethodPost:
		var body struct{ App, Host string }
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		app := sanitizeName(body.App)
		if !p.canAccess(r, app) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		host, ok := normalizeHost(body.Host, p.domain)
		if !ok {
			http.Error(w, "enter a valid domain (not a "+p.domain+" subdomain)", http.StatusBadRequest)
			return
		}
		if _, taken := domainOwner(host); taken {
			http.Error(w, "that domain is already in use", http.StatusConflict)
			return
		}
		if _, err := sqlDB.Exec(
			`INSERT INTO domains(host,app,team,created) VALUES(?,?,?,?)`,
			host, app, p.teamID(r), time.Now().Unix()); err != nil {
			http.Error(w, "could not add domain", http.StatusInternalServerError)
			return
		}
		writeDomainsFile()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Domain{Host: host, App: app, Created: time.Now().Unix()})

	case http.MethodDelete:
		host, ok := normalizeHost(r.URL.Query().Get("host"), p.domain)
		if !ok {
			http.Error(w, "bad host", http.StatusBadRequest)
			return
		}
		owner, exists := domainOwner(host)
		if !exists {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !p.canAccess(r, owner.App) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = sqlDB.Exec(`DELETE FROM domains WHERE host=?`, host)
		writeDomainsFile()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// resolveDomains records where each host currently resolves and whether that
// includes this server's public IP (i.e. DNS is pointed here).
func resolveDomains(domains []Domain, serverIP string) {
	for i := range domains {
		ips := resolveIPs(domains[i].Host)
		domains[i].ResolvesTo = sortedIPs(ips)
		if serverIP != "" && ips[serverIP] {
			domains[i].PointsHere = true
		}
	}
}

func resolveIPs(host string) map[string]bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		set[a] = true
	}
	return set
}

func sortedIPs(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for ip := range set {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

var (
	pubIPMu  sync.Mutex
	pubIPVal string
)

// serverPublicIP is this box's public IP — the value a custom domain's A record
// should point at. Determined once via an egress lookup, falling back to the
// base domain's own A record (which points here in the standard setup).
func serverPublicIP(base string) string {
	pubIPMu.Lock()
	defer pubIPMu.Unlock()
	if pubIPVal != "" {
		return pubIPVal
	}
	if ip := egressIP(); ip != "" {
		pubIPVal = ip
	} else {
		for ip := range resolveIPs(base) {
			pubIPVal = ip
			break
		}
	}
	return pubIPVal
}

func egressIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	ip := strings.TrimSpace(string(b))
	if net.ParseIP(ip) != nil {
		return ip
	}
	return ""
}
