package panel

import (
	"net"
	"testing"
)

func TestDialFilterCoversSharedAndTranslatedRanges(t *testing.T) {
	for _, s := range []string{"100.64.0.1", "100.127.255.254", "192.0.0.8", "64:ff9b::7f00:1", "2002:7f00:1::1"} {
		if !isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"100.128.0.1", "192.0.2.1", "2606:4700::1"} {
		if isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}
