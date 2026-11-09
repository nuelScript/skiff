package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// Permissions are what a container may do beyond Docker's defaults. Each one weakens the wall between the app and the server, so the server's admin grants them per app.
type Permissions struct {
	// Capabilities are Linux capabilities to add, like NET_ADMIN.
	Capabilities []string `json:"capabilities,omitempty" toml:"cap_add"`
	// Devices are host devices to pass in, like /dev/net/tun.
	Devices []string `json:"devices,omitempty" toml:"devices"`
	// Sysctls are kernel settings inside the container's own namespace, as key=value.
	Sysctls []string `json:"sysctls,omitempty" toml:"sysctls"`
	// Privileged gives the container nearly everything the server has. It's the escape hatch for software that needs it.
	Privileged bool `json:"privileged,omitempty" toml:"privileged"`
}

func (p Permissions) IsZero() bool { return reflect.ValueOf(p).IsZero() }

func (p Permissions) Equal(o Permissions) bool { return reflect.DeepEqual(p, o) }

// StrongCapabilities let a container reach past its own namespace, close to what privileged gives.
var StrongCapabilities = []string{"SYS_ADMIN", "SYS_MODULE", "SYS_RAWIO", "SYS_BOOT", "SYS_TIME", "MAC_ADMIN", "MAC_OVERRIDE", "DAC_READ_SEARCH", "BPF", "PERFMON", "SYSLOG"}

var (
	capabilityPattern = regexp.MustCompile(`^[A-Z][A-Z_]{1,30}$`)
	devicePattern     = regexp.MustCompile(`^/dev/[A-Za-z0-9._/-]+(:/dev/[A-Za-z0-9._/-]+)?(:[rwm]{1,3})?$`)
	sysctlPattern     = regexp.MustCompile(`^[a-z0-9_.]+=[A-Za-z0-9 _./:-]*$`)
	// namespacedSysctls are the ones Docker can set for one container without touching the server's own.
	namespacedSysctls = []string{"net.", "kernel.shm", "kernel.msg", "kernel.sem", "fs.mqueue."}
)

// Normalize upper-cases capabilities, drops their CAP_ prefix, and sorts and dedupes each list, so two ways of writing the same grant compare equal.
func (p Permissions) Normalize() Permissions {
	out := p
	out.Capabilities = nil
	for _, c := range p.Capabilities {
		out.Capabilities = append(out.Capabilities, strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_"))
	}
	clean := func(in []string) []string {
		var r []string
		for _, s := range in {
			if s = strings.TrimSpace(s); s != "" {
				r = append(r, s)
			}
		}
		slices.Sort(r)
		return slices.Compact(r)
	}
	out.Capabilities, out.Devices, out.Sysctls = clean(out.Capabilities), clean(p.Devices), clean(p.Sysctls)
	return out
}

// Validate refuses values Docker would misread or that would reach the server's own settings.
func (p Permissions) Validate() error {
	for _, c := range p.Capabilities {
		if !capabilityPattern.MatchString(c) || c == "ALL" {
			return fmt.Errorf("capability %q isn't one Docker knows: use a name like NET_ADMIN", c)
		}
	}
	for _, d := range p.Devices {
		if !devicePattern.MatchString(d) || strings.Contains(d, "..") {
			return fmt.Errorf("device %q must be a path under /dev, like /dev/net/tun", d)
		}
	}
	for _, s := range p.Sysctls {
		if !sysctlPattern.MatchString(s) {
			return fmt.Errorf("sysctl %q must look like net.ipv4.ip_forward=1", s)
		}
		key, _, _ := strings.Cut(s, "=")
		if !slices.ContainsFunc(namespacedSysctls, func(prefix string) bool { return strings.HasPrefix(key, prefix) }) {
			return fmt.Errorf("sysctl %s would change the whole server; only net.*, kernel.shm*, kernel.msg*, kernel.sem and fs.mqueue.* can be set for one app", key)
		}
	}
	return nil
}

// Strong reports whether the grant reaches past the container's own namespace.
func (p Permissions) Strong() bool {
	return p.Privileged || slices.ContainsFunc(p.Capabilities, func(c string) bool { return slices.Contains(StrongCapabilities, c) })
}

// DockerArgs are the flags for docker run.
func (p Permissions) DockerArgs() []string {
	var args []string
	if p.Privileged {
		args = append(args, "--privileged")
	}
	for _, c := range p.Capabilities {
		args = append(args, "--cap-add", c)
	}
	for _, d := range p.Devices {
		args = append(args, "--device", d)
	}
	for _, s := range p.Sysctls {
		args = append(args, "--sysctl", s)
	}
	return args
}

// EncodePermissions stores permissions as JSON, empty when there are none.
func EncodePermissions(p Permissions) string {
	if p.IsZero() {
		return ""
	}
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodePermissions reads what EncodePermissions stored; anything unreadable grants nothing.
func DecodePermissions(s string) Permissions {
	var p Permissions
	if s == "" || json.Unmarshal([]byte(s), &p) != nil {
		return Permissions{}
	}
	return p
}
