package panel

import (
	"testing"

	"github.com/nuelScript/skiff/internal/compose"
)

func TestGithubRepo(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/acme/shop":      "acme/shop",
		"https://github.com/acme/shop.git":  "acme/shop",
		"https://github.com/acme/shop/":     "acme/shop",
		"https://gitlab.com/acme/shop":      "",
		"https://github.com/acme":           "",
		"https://github.com/acme/shop/tree": "",
	} {
		if got := githubRepo(in); got != want {
			t.Errorf("%s = %q, want %q", in, got, want)
		}
	}
}

func TestStackAppsFollowPushes(t *testing.T) {
	openTestDB(t)
	s := &stackSource{cloneURL: "https://github.com/acme/shop", branch: "main", dir: "/deploy",
		plan: &compose.Plan{Stack: "shop"}}
	built := stackSourceFor(s, "t1", compose.Item{Service: "api", Name: "shop-api", Build: &compose.Build{Context: "../api", Dockerfile: "Dockerfile.prod"}, Port: 3000, Public: true})
	pulled := stackSourceFor(s, "t1", compose.Item{Service: "search", Name: "shop-search", Image: "meili:1", Port: 7700})
	for _, src := range []Source{built, pulled} {
		if err := putSource(src); err != nil {
			t.Fatal(err)
		}
	}
	if built.RootDir != "api" || built.Dockerfile != "Dockerfile.prod" || built.HealthMode != "" {
		t.Errorf("built: %+v", built)
	}
	if pulled.HealthMode != "tcp" || !pulled.Private || pulled.Image != "meili:1" {
		t.Errorf("pulled: %+v", pulled)
	}
	got := sourcesForRepo("acme/shop", "main")
	if len(got) != 1 || got[0].App != "shop-api" {
		t.Fatalf("a push should redeploy the built app only: %+v", got)
	}
	back, ok := getSource("shop-api")
	if !ok || back.Stack != "shop" || back.Service != "api" {
		t.Fatalf("stored: %+v", back)
	}
}
