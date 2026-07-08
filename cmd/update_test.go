package cmd

import (
	"strings"
	"testing"
)

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.1.0", "0.1.1", true},
		{"0.1.1", "0.1.1", false},
		{"0.1.1", "0.1.0", false},
		{"0.1.0", "0.2.0", true},
		{"0.9.9", "1.0.0", true},
		{"1.2.3", "1.2.10", true},     // numeric, not lexical ("3" < "10")
		{"v0.1.0", "v0.1.1", true},    // leading v tolerated
		{"0.1.1-rc1", "0.1.1", false}, // pre-release suffix ignored → equal
		{"0.1.0", "0.1.1-rc1", true},  // ...but a lower base still updates
		{"1.0.0", "0.9.9", false},     // major dominates
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestUpdateNotice(t *testing.T) {
	msg := updateNotice("0.1.0", "0.1.1")
	for _, want := range []string{"0.1.1", "0.1.0", updateInstallCmd} {
		if !strings.Contains(msg, want) {
			t.Fatalf("notice %q missing %q", msg, want)
		}
	}
	if got := updateNotice("0.1.1", "0.1.1"); got != "" {
		t.Errorf("up-to-date should be silent, got %q", got)
	}
	if got := updateNotice("0.1.1", ""); got != "" {
		t.Errorf("unknown latest should be silent, got %q", got)
	}
}
