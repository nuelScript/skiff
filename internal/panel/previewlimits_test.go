package panel

import (
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/egress"
)

func TestPreviewsKeepTheirParentsLimits(t *testing.T) {
	openTestDB(t)
	locked := egress.Policy{Mode: egress.List, Allow: []string{"api.stripe.com"}}.Encode()
	putSource(Source{App: "web", Team: "t1", Port: "3000", Isolation: "strong", Egress: locked, Permissions: `{"capabilities":["NET_ADMIN"]}`})
	putSource(Source{App: "web-feat", Team: "t1", Port: "3000", Parent: "web"})

	pv, _ := getSource("web-feat")
	got := withParentLimits(pv)
	if got.Isolation != "strong" || got.Egress != locked {
		t.Fatalf("preview runs with isolation %q egress %q", got.Isolation, got.Egress)
	}
	if got.Permissions != "" {
		t.Fatalf("preview was given production's machine permissions: %s", got.Permissions)
	}
	toml := projectToml(got, nil, "net", nil)
	if !strings.Contains(toml, `isolation = "strong"`) || !strings.Contains(toml, "egress_allow") {
		t.Fatalf("deploy config is missing the limits:\n%s", toml)
	}

	// Loosening production loosens its previews on their next deploy.
	putSource(Source{App: "web", Team: "t1", Port: "3000"})
	if got := withParentLimits(pv); got.Isolation != "" || egress.Decode(got.Egress).Restricted() {
		t.Fatalf("preview kept limits production dropped: %+v", got)
	}
}

func TestStackPreviewsKeepTheirServicesLimits(t *testing.T) {
	openTestDB(t)
	locked := egress.Policy{Mode: egress.Private}.Encode()
	for _, st := range []struct{ name, parent string }{{"shop", ""}, {"shop-feat", "shop"}} {
		if _, err := sqlDB.Exec(`INSERT INTO stacks(name,team,created,vars,parent) VALUES(?,'t1',1,'',?)`, st.name, st.parent); err != nil {
			t.Fatal(err)
		}
	}
	putSource(Source{App: "shop-api", Team: "t1", Stack: "shop", Service: "api", Isolation: "strong", Egress: locked})
	putSource(Source{App: "shop-feat-api", Team: "t1", Stack: "shop-feat", Service: "api"})
	putSource(Source{App: "shop-feat-web", Team: "t1", Stack: "shop-feat", Service: "web"})

	api, _ := getSource("shop-feat-api")
	if got := withParentLimits(api); got.Isolation != "strong" || got.Egress != locked {
		t.Fatalf("stack preview service runs with %q %q", got.Isolation, got.Egress)
	}
	web, _ := getSource("shop-feat-web")
	if got := withParentLimits(web); got.Isolation != "" || got.Egress != "" {
		t.Fatalf("a service production doesn't limit was limited: %+v", got)
	}
}
