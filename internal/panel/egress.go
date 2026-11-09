package panel

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/egress"
)

// egressLoop keeps the server's outbound rules matching the running containers: right after one starts or stops, and every half minute in case an event was missed.
func (p *Panel) egressLoop() {
	if !egress.Available() {
		return
	}
	wake := make(chan struct{}, 1)
	go func() {
		for {
			_ = p.eng.WatchContainers(context.Background(), func() {
				select {
				case wake <- struct{}{}:
				default:
				}
			})
			time.Sleep(5 * time.Second)
		}
	}()
	applied := ""
	for {
		guard("egressLoop", func() {
			rules, ok := p.egressRules()
			if !ok || rules == applied {
				return
			}
			if err := egress.Apply(rules); err != nil {
				log.Printf("skiff: applying outbound rules: %v", err)
				return
			}
			applied = rules
		})
		select {
		case <-wake:
		case <-time.After(30 * time.Second):
		}
	}
}

// egressRules builds the rules from every running container on Skiff's networks, with the policy its label carries.
func (p *Panel) egressRules() (string, bool) {
	addrs, err := p.eng.ContainerAddrs()
	if err != nil {
		return "", false
	}
	byNet := map[string]*egress.Network{}
	var names []string
	for _, a := range addrs {
		policy := egress.Decode(a.Labels[egress.Label])
		for name, ip := range a.Networks {
			if !strings.HasPrefix(name, "skiff") {
				continue
			}
			n := byNet[name]
			if n == nil {
				subnet := subnetOf(p, name)
				if subnet == "" {
					continue
				}
				n = &egress.Network{Subnet: subnet}
				byNet[name] = n
				names = append(names, name)
			}
			n.Members = append(n.Members, egress.Member{IP: ip, Policy: policy})
		}
	}
	slices.Sort(names)
	nets := make([]egress.Network, 0, len(names))
	for _, name := range names {
		n := byNet[name]
		slices.SortFunc(n.Members, func(a, b egress.Member) int { return strings.Compare(a.IP, b.IP) })
		nets = append(nets, *n)
	}
	return egress.Rules(nets, resolveHost), true
}

var subnets sync.Map // network name → subnet; a network's range doesn't change while it exists

func subnetOf(p *Panel, name string) string {
	if v, ok := subnets.Load(name); ok {
		return v.(string)
	}
	s := p.eng.NetworkSubnet(name)
	if s != "" {
		subnets.Store(name, s)
	}
	return s
}

// resolveHost looks a listed host up at most every five minutes. Hosts behind changing addresses work best as ranges.
var hostCache = struct {
	sync.Mutex
	ips map[string][]string
	at  map[string]time.Time
}{ips: map[string][]string{}, at: map[string]time.Time{}}

func resolveHost(host string) []string {
	hostCache.Lock()
	defer hostCache.Unlock()
	if time.Since(hostCache.at[host]) < 5*time.Minute {
		return hostCache.ips[host]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err == nil {
		hostCache.ips[host], hostCache.at[host] = ips, time.Now()
	}
	return hostCache.ips[host]
}

func egressLabels(src Source) map[string]string {
	return map[string]string{egress.Label: egress.Decode(src.Egress).Encode()}
}

// handleEgress shows and sets where an app may connect. Limiting only takes away, so any team owner can.
func (p *Panel) handleEgress(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"policy": egress.Decode(src.Egress), "available": egress.Available()})
	case http.MethodPut:
		if !p.isOwner(r) {
			http.Error(w, "only a team owner can change where an app may connect", http.StatusForbidden)
			return
		}
		var body egress.Policy
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body = body.Normalize()
		if err := body.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Restricted() && !egress.Available() {
			http.Error(w, "this server can't limit outbound connections: it needs Linux, with Skiff running as root", http.StatusBadRequest)
			return
		}
		src.Egress = body.Encode()
		if err := putSource(src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.audit(r, "project.egress", app, describeEgress(body))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func describeEgress(p egress.Policy) string {
	switch p.Mode {
	case egress.Private:
		return "private only"
	case egress.List:
		return "private and " + strings.Join(p.Allow, ", ")
	}
	return "anywhere"
}
