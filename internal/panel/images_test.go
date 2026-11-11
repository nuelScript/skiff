package panel

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/seal"
)

func TestImageProjectToml(t *testing.T) {
	for _, port := range []string{"", "9000"} {
		src := Source{App: "kuma", Image: "louislam/uptime-kuma:1", Port: port}
		out := projectToml(src, nil, teamNetwork(""), nil)
		path := filepath.Join(t.TempDir(), "skiff.toml")
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatalf("generated toml doesn't parse: %v\n%s", err, out)
		}
		if cfg.Build.Image != src.Image {
			t.Fatalf("image = %q", cfg.Build.Image)
		}
		// No port means the deploy reads it from the image.
		want := 0
		if port != "" {
			want = 9000
		}
		if cfg.Build.Port != want {
			t.Fatalf("port %q rendered as %d", port, cfg.Build.Port)
		}
	}
}

func TestRegistryCreds(t *testing.T) {
	openTestDB(t)
	if err := putSource(Source{App: "api", Image: "ghcr.io/acme/api:1"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := getSource("api"); got.Image != "ghcr.io/acme/api:1" {
		t.Fatalf("image didn't round-trip: %q", got.Image)
	}
	if _, _, _, ok := getRegistryCreds("api"); ok {
		t.Fatal("credentials before any were set")
	}
	_ = setRegistryCreds("api", "ghcr.io", "bot", "one")
	_ = setRegistryCreds("api", "ghcr.io", "bot", "two")
	if host, u, pw, ok := getRegistryCreds("api"); !ok || host != "ghcr.io" || u != "bot" || pw != "two" {
		t.Fatalf("creds = %q %q %q %v", host, u, pw, ok)
	}
	if u, pw, ok := pullLogin("api", "ghcr.io/acme/api:2"); !ok || u != "bot" || pw != "two" {
		t.Errorf("login for the same registry: %q %q %v", u, pw, ok)
	}
	// Pointing the project at another registry must not hand that registry the login.
	if _, _, ok := pullLogin("api", "evil.example.com/acme/api:1"); ok {
		t.Error("the login was offered to another registry")
	}
	deleteSource("api")
	if _, _, _, ok := getRegistryCreds("api"); ok {
		t.Fatal("deleting the project left its registry credentials behind")
	}
}

// A login saved before hosts were kept takes its app's current image host at startup, and is never offered before that.
func TestOldRegistryLoginTakesItsImagesHost(t *testing.T) {
	openTestDB(t)
	if err := putSource(Source{App: "api", Image: "ghcr.io/acme/api:1"}); err != nil {
		t.Fatal(err)
	}
	sealed, _ := seal.Seal("pw")
	if _, err := sqlDB.Exec(`INSERT INTO registry_creds(app,username,password) VALUES('api','bot',?)`, sealed); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := pullLogin("api", "registry.example.com/x:1"); ok {
		t.Fatal("a host-less login was offered to a new registry")
	}
	if _, _, ok := pullLogin("api", "ghcr.io/acme/api:1"); ok {
		t.Fatal("a host-less login was used before the backfill")
	}
	backfillRegistryHosts()
	if _, _, ok := pullLogin("api", "ghcr.io/acme/api:1"); !ok {
		t.Fatal("the backfilled login wasn't used for its own image")
	}
	if _, _, ok := pullLogin("api", "registry.example.com/x:1"); ok {
		t.Error("the login went to another registry")
	}
}

func TestImageProjectValidation(t *testing.T) {
	openTestDB(t)
	p := &Panel{}
	_ = putSource(Source{App: "taken", Repo: "acme/taken", Port: "3000"})
	for body, want := range map[string]int{
		`{"name":"Bad Name","image":"nginx"}`:             http.StatusBadRequest,
		`{"name":"web","image":"nginx latest"}`:           http.StatusBadRequest,
		`{"name":"web","image":"nginx","port":"99999"}`:   http.StatusBadRequest,
		`{"name":"web","image":"nginx","username":"bot"}`: http.StatusBadRequest,
		`{"name":"taken","image":"nginx"}`:                http.StatusConflict,
	} {
		w := httptest.NewRecorder()
		p.handleImageProject(w, httptest.NewRequest("POST", "/api/projects/image", strings.NewReader(body)))
		if w.Code != want {
			t.Errorf("%s → %d, want %d (%s)", body, w.Code, want, strings.TrimSpace(w.Body.String()))
		}
	}
	if got, _ := getSource("taken"); got.Image != "" {
		t.Fatal("an existing project was turned into an image project")
	}
}
