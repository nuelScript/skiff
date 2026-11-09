package egress

import (
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	resolve := func(h string) []string {
		return map[string][]string{"api.example.com": {"203.0.113.7", "2001:db8::1"}}[h]
	}
	nets := []Network{
		{Subnet: "172.20.0.0/16", Members: []Member{
			{IP: "172.20.0.2"},
			{IP: "172.20.0.3", Policy: Policy{Mode: Private}},
			{IP: "172.20.0.4", Policy: Policy{Mode: List, Allow: []string{"api.example.com", "198.51.100.0/24"}}},
		}},
		{Subnet: "172.21.0.0/16", Members: []Member{{IP: "172.21.0.2"}}},
	}
	got := Rules(nets, resolve)
	for _, want := range []string{
		"-A SKIFF-EGRESS -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN",
		"-A SKIFF-EGRESS -s 172.20.0.0/16 -d 10.0.0.0/8 -j RETURN",
		"-A SKIFF-EGRESS -s 172.20.0.2/32 -j RETURN",
		"-A SKIFF-EGRESS -s 172.20.0.4/32 -d 203.0.113.7/32 -j RETURN",
		"-A SKIFF-EGRESS -s 172.20.0.4/32 -d 198.51.100.0/24 -j RETURN",
		"-A SKIFF-EGRESS -s 172.20.0.0/16 -p tcp -j REJECT --reject-with tcp-reset",
	} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "172.20.0.3/32") {
		t.Error("a private-only container shouldn't get its own allow rule")
	}
	if strings.Contains(got, "172.21.0.0/16") {
		t.Error("a network with nothing restricted should be left alone")
	}
	if strings.Contains(got, "2001:db8") {
		t.Error("IPv6 addresses aren't written into the IPv4 rules")
	}
	if !strings.HasPrefix(got, "*filter\n:SKIFF-EGRESS - [0:0]\n") || !strings.HasSuffix(got, "COMMIT\n") {
		t.Errorf("not a whole iptables-restore table:\n%s", got)
	}
}

func TestPolicies(t *testing.T) {
	for _, bad := range []Policy{{Mode: "nowhere"}, {Mode: List}, {Mode: List, Allow: []string{"bad host"}}, {Mode: List, Allow: []string{"-j ACCEPT"}}} {
		if bad.Validate() == nil {
			t.Errorf("should be refused: %+v", bad)
		}
	}
	p := Policy{Mode: List, Allow: []string{" API.Example.com", "api.example.com", "10.0.0.0/8"}}.Normalize()
	if p.Validate() != nil || len(p.Allow) != 2 || p.Allow[1] != "api.example.com" {
		t.Errorf("normalized = %+v", p)
	}
	if Decode(p.Encode()).Mode != List || Decode("{not json").Mode != Private || Decode("").Restricted() {
		t.Error("decode")
	}
	if (Policy{Mode: Private, Allow: []string{"x"}}).Normalize().Allow != nil {
		t.Error("private mode drops the list")
	}
}
