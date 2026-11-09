package config

import (
	"strings"
	"testing"
)

func TestHostMemoryAndDisksAreRefused(t *testing.T) {
	for _, d := range []string{"/dev/mem", "/dev/kmem", "/dev/sda", "/dev/sda1", "/dev/nvme0n1", "/dev/nvme0n1p2", "/dev/vdb", "/dev/xvda1", "/dev/mmcblk0p1", "/dev/dm-0", "/dev/mapper/root", "/dev/disk/by-id/x", "/dev/loop3", "/dev/md0", "/dev/sda:/dev/xvdz"} {
		if err := (Permissions{Devices: []string{d}}).Validate(); err == nil || !strings.Contains(err.Error(), "memory or one of its disks") {
			t.Errorf("%s: %v", d, err)
		}
	}
	for _, d := range []string{"/dev/net/tun", "/dev/fuse", "/dev/snd", "/dev/dri/renderD128", "/dev/ttyUSB0", "/dev/kvm"} {
		if err := (Permissions{Devices: []string{d}}).Validate(); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
}

func TestADeviceIsAStrongGrant(t *testing.T) {
	if !(Permissions{Devices: []string{"/dev/net/tun"}}).Strong() {
		t.Error("a device grant should count as strong")
	}
	if (Permissions{Capabilities: []string{"NET_ADMIN"}}).Strong() {
		t.Error("NET_ADMIN alone isn't strong")
	}
}
