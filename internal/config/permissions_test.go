package config

import (
	"slices"
	"testing"
)

func TestPermissions(t *testing.T) {
	p := Permissions{Capabilities: []string{" cap_net_admin", "NET_ADMIN"}, Devices: []string{"/dev/net/tun"}, Sysctls: []string{"net.core.somaxconn=1024"}}.Normalize()
	if !slices.Equal(p.Capabilities, []string{"NET_ADMIN"}) {
		t.Errorf("capabilities = %v", p.Capabilities)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if !p.Strong() {
		t.Error("a device makes the grant strong")
	}
	if got := p.DockerArgs(); !slices.Equal(got, []string{"--cap-add", "NET_ADMIN", "--device", "/dev/net/tun", "--sysctl", "net.core.somaxconn=1024"}) {
		t.Errorf("args = %v", got)
	}
	for _, bad := range []Permissions{
		{Capabilities: []string{"ALL"}},
		{Capabilities: []string{"--privileged"}},
		{Devices: []string{"/etc/passwd"}},
		{Devices: []string{"/dev/../etc/shadow"}},
		{Sysctls: []string{"kernel.panic=1"}},
		{Sysctls: []string{"net.core.somaxconn"}},
	} {
		if bad.Normalize().Validate() == nil {
			t.Errorf("should be refused: %+v", bad)
		}
	}
	if !(Permissions{Privileged: true}).Strong() || !(Permissions{Capabilities: []string{"SYS_ADMIN"}}).Strong() {
		t.Error("privileged and SYS_ADMIN are strong")
	}
	if got := DecodePermissions(EncodePermissions(p)); !got.Equal(p) {
		t.Errorf("round trip: %+v", got)
	}
}

func TestSaferDefaults(t *testing.T) {
	if got := (Permissions{}).HardeningArgs(); !slices.Equal(got, []string{"--security-opt", "no-new-privileges", "--cap-drop", "NET_RAW", "--cap-drop", "MKNOD", "--cap-drop", "AUDIT_WRITE"}) {
		t.Errorf("defaults = %v", got)
	}
	if got := (Permissions{Capabilities: []string{"NET_RAW"}, NewPrivileges: true}).HardeningArgs(); !slices.Equal(got, []string{"--cap-drop", "MKNOD", "--cap-drop", "AUDIT_WRITE"}) {
		t.Errorf("a grant gives its capability back: %v", got)
	}
	if got := (Permissions{Privileged: true}).HardeningArgs(); got != nil {
		t.Errorf("privileged gets everything: %v", got)
	}
}
