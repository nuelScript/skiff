// Package egress limits where containers may connect to, with a chain of the server's firewall rules.
package egress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// Label is the container label that carries a policy, so the rules follow the containers rather than a list kept elsewhere.
const Label = "skiff.egress"

// Chain is Skiff's own chain, jumped to from Docker's DOCKER-USER, which Docker leaves to the server's owner.
const Chain = "SKIFF-EGRESS"

// Modes of a policy. An empty mode connects anywhere.
const (
	Private = "private" // only private addresses: the team's apps and databases, no internet
	List    = "list"    // private addresses and the listed hosts
)

// MaxAllow bounds how many hosts one policy lists.
const MaxAllow = 50

// privateRanges are the addresses a restricted container may always reach: other containers, the team's databases, the server's private network.
var privateRanges = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

// Policy is where one app's containers may connect.
type Policy struct {
	Mode  string   `json:"mode,omitempty"`
	Allow []string `json:"allow,omitempty"`
}

func (p Policy) Restricted() bool { return p.Mode == Private || p.Mode == List }

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// Normalize trims, lower-cases and dedupes the hosts, and drops them when the mode doesn't use them.
func (p Policy) Normalize() Policy {
	out := Policy{Mode: p.Mode}
	if p.Mode != List {
		return out
	}
	for _, a := range p.Allow {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && !slices.Contains(out.Allow, a) {
			out.Allow = append(out.Allow, a)
		}
	}
	slices.Sort(out.Allow)
	return out
}

// Validate refuses a mode Skiff doesn't know and hosts that aren't a name, an address or a range.
func (p Policy) Validate() error {
	switch p.Mode {
	case "", Private:
		return nil
	case List:
	default:
		return fmt.Errorf("outbound mode %q isn't one Skiff knows: use private or list", p.Mode)
	}
	if len(p.Allow) == 0 {
		return errors.New("list at least one host it may reach")
	}
	if len(p.Allow) > MaxAllow {
		return fmt.Errorf("list at most %d hosts", MaxAllow)
	}
	for _, a := range p.Allow {
		if _, _, err := net.ParseCIDR(a); err == nil {
			continue
		}
		if net.ParseIP(a) != nil || hostPattern.MatchString(a) {
			continue
		}
		return fmt.Errorf("%q isn't a host name, an address or a range like 203.0.113.0/24", a)
	}
	return nil
}

// Encode is the label value for a policy, empty when it isn't restricted.
func (p Policy) Encode() string {
	if !p.Restricted() {
		return ""
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// Decode reads a label; anything unreadable is treated as private, since a policy someone meant to set is safer closed than open.
func Decode(label string) Policy {
	if label == "" {
		return Policy{}
	}
	var p Policy
	if json.Unmarshal([]byte(label), &p) != nil || p.Validate() != nil {
		return Policy{Mode: Private}
	}
	return p
}

// Member is one container on a network, with its address and policy.
type Member struct {
	IP     string
	Policy Policy
}

// Network is one of Skiff's container networks.
type Network struct {
	Subnet  string
	Members []Member
}

// Rules writes the chain for iptables-restore. A network with no restricted container gets no rules, so nothing changes for teams that don't use this. On a network that has one, traffic to the internet is refused unless its container is known and allowed: a container started a moment ago, before the rules know it, can't get out either.
func Rules(networks []Network, resolve func(host string) []string) string {
	var b bytes.Buffer
	b.WriteString("*filter\n:" + Chain + " - [0:0]\n")
	fmt.Fprintf(&b, "-A %s -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN\n", Chain)
	for _, n := range networks {
		if !slices.ContainsFunc(n.Members, func(m Member) bool { return m.Policy.Restricted() }) {
			continue
		}
		for _, r := range privateRanges {
			fmt.Fprintf(&b, "-A %s -s %s -d %s -j RETURN\n", Chain, n.Subnet, r)
		}
		for _, m := range n.Members {
			switch m.Policy.Mode {
			case "":
				fmt.Fprintf(&b, "-A %s -s %s/32 -j RETURN\n", Chain, m.IP)
			case List:
				for _, d := range destinations(m.Policy.Allow, resolve) {
					fmt.Fprintf(&b, "-A %s -s %s/32 -d %s -j RETURN\n", Chain, m.IP, d)
				}
			}
		}
		fmt.Fprintf(&b, "-A %s -s %s -p tcp -j REJECT --reject-with tcp-reset\n", Chain, n.Subnet)
		fmt.Fprintf(&b, "-A %s -s %s -j REJECT\n", Chain, n.Subnet)
	}
	b.WriteString("COMMIT\n")
	return b.String()
}

// destinations turns hosts into addresses and ranges, IPv4 only, like Docker's networks.
func destinations(allow []string, resolve func(string) []string) []string {
	var out []string
	add := func(d string) {
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	for _, a := range allow {
		if _, n, err := net.ParseCIDR(a); err == nil {
			if n.IP.To4() != nil {
				add(n.String())
			}
			continue
		}
		if ip := net.ParseIP(a); ip != nil {
			if ip.To4() != nil {
				add(ip.String() + "/32")
			}
			continue
		}
		for _, ip := range resolve(a) {
			if p := net.ParseIP(ip); p != nil && p.To4() != nil {
				add(p.String() + "/32")
			}
		}
	}
	return out
}

// Available reports whether this server can enforce policies: Linux, as root, with iptables.
func Available() bool {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return false
	}
	_, err := exec.LookPath("iptables-restore")
	return err == nil
}

// Apply replaces the chain in one step, so there's never a moment with half the rules, and makes sure Docker's traffic passes through it.
// InPlace reports whether Skiff's chain still exists and DOCKER-USER still jumps to it. A firewall reload or a Docker restart can remove either.
func InPlace() bool {
	return exec.Command("iptables", "-w", "-n", "-L", Chain).Run() == nil &&
		exec.Command("iptables", "-w", "-C", "DOCKER-USER", "-j", Chain).Run() == nil
}

func Apply(rules string) error {
	cmd := exec.Command("iptables-restore", "--noflush", "-w")
	cmd.Stdin = strings.NewReader(rules)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables-restore: %s", strings.TrimSpace(string(out)))
	}
	if exec.Command("iptables", "-w", "-C", "DOCKER-USER", "-j", Chain).Run() != nil {
		if out, err := exec.Command("iptables", "-w", "-I", "DOCKER-USER", "1", "-j", Chain).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables: %s", strings.TrimSpace(string(out)))
		}
	}
	return nil
}
