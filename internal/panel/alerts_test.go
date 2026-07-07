package panel

import (
	"net"
	"testing"
)

func TestIsBlockedDialIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"::1",             // loopback v6
		"10.0.0.5",        // private
		"172.16.4.4",      // private
		"192.168.1.1",     // private
		"169.254.169.254", // link-local (cloud metadata)
		"0.0.0.0",         // unspecified
		"fc00::1",         // unique-local v6
	}
	for _, s := range blocked {
		if !isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked for outbound alerts", s)
		}
	}
	if !isBlockedDialIP(nil) {
		t.Error("an unparseable address must be blocked")
	}

	allowed := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:2800:220:1::"}
	for _, s := range allowed {
		if isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("public address %s should be allowed", s)
		}
	}
}
