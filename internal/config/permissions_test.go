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
	if p.Strong() {
		t.Error("NET_ADMIN isn't one of the strong ones")
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
