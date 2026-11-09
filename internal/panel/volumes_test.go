package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/config"
)

func TestVolumeDefs(t *testing.T) {
	openTestDB(t)
	p := &Panel{}
	for _, s := range []Source{
		{App: "shop", Repo: "acme/shop", Branch: "main", Port: "3000"},
		{App: "shop-feat-x", Repo: "acme/shop", Branch: "feat/x", Parent: "shop", Port: "3000"},
	} {
		if err := putSource(s); err != nil {
			t.Fatal(err)
		}
	}
	data := config.Volume{Name: "data", Path: "/data"}
	if err := addVolume("shop", data); err != nil {
		t.Fatal(err)
	}
	if addVolume("shop", config.Volume{Name: "data", Path: "/other"}) == nil {
		t.Fatal("a second volume with the same name was accepted")
	}

	shop, _ := getSource("shop")
	preview, _ := getSource("shop-feat-x")
	if got := p.volumeDefs(shop); len(got) != 1 || !got[0].Equal(data) {
		t.Fatalf("project defs = %+v", got)
	}
	// A preview mounts the same paths, but on volumes named after itself, so it never touches production's files.
	if got := p.volumeDefs(preview); len(got) != 1 || !got[0].Equal(data) {
		t.Fatalf("preview defs = %+v", got)
	}
	out := projectToml(preview, nil, teamNetwork(""), p.volumeDefs(preview))
	path := filepath.Join(t.TempDir(), "skiff.toml")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("generated toml doesn't parse: %v\n%s", err, out)
	}
	if len(cfg.Volumes) != 1 || config.VolumeName(cfg.Name, cfg.Volumes[0].Name) != "skiff-shop-feat-x_data" {
		t.Fatalf("preview volumes = %+v", cfg.Volumes)
	}

	if !deleteVolume("shop", "data") || deleteVolume("shop", "data") {
		t.Fatal("deleteVolume should report whether it removed a row")
	}
	_ = addVolume("shop", data)
	deleteSource("shop")
	if len(listVolumes("shop")) != 0 {
		t.Fatal("deleting the project left its volumes behind")
	}
}

func TestNewVolume(t *testing.T) {
	openTestDB(t)
	if err := addVolume("shop", config.Volume{Name: "data", Path: "/data"}); err != nil {
		t.Fatal(err)
	}
	v, problem := newVolume("shop", config.Volume{Name: " uploads ", Path: " /uploads ", Owner: "someone-else"})
	if problem != "" || v.Name != "uploads" || v.Path != "/uploads" {
		t.Fatalf("newVolume = %+v, %q", v, problem)
	}
	if v.Owner != "" {
		t.Errorf("a volume added by hand belongs to %q, not its own app", v.Owner)
	}
	if _, problem := newVolume("shop", config.Volume{Name: "data", Path: "/elsewhere"}); problem == "" {
		t.Error("a second volume named data was accepted")
	}
}

func volumeCall(p *Panel, session, method, query, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/volumes?"+query, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "skiff_session", Value: session})
	w := httptest.NewRecorder()
	if strings.HasPrefix(query, "shared&") {
		p.handleSharedVolumes(w, req)
	} else {
		p.handleVolumes(w, req)
	}
	return w
}

func TestSharedVolumes(t *testing.T) {
	p, team, other := twoTeams(t)
	for _, s := range []Source{
		{App: "ollama", Team: team, Repo: "acme/ollama", Branch: "main"},
		{App: "chat", Team: team, Repo: "acme/chat", Branch: "main"},
		{App: "chat-feat", Team: team, Repo: "acme/chat", Branch: "feat", Parent: "chat"},
		{App: "theirs", Team: other, Repo: "x/theirs", Branch: "main"},
	} {
		if err := putSource(s); err != nil {
			t.Fatal(err)
		}
	}
	_ = addVolume("ollama", config.Volume{Name: "models", Path: "/root/.ollama"})
	_ = addVolume("theirs", config.Volume{Name: "secret", Path: "/data"})

	w := volumeCall(p, "founder", "GET", "shared&app=chat", "")
	var list []sharedVolume
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 || list[0] != (sharedVolume{App: "ollama", Name: "models", Path: "/root/.ollama"}) {
		t.Fatalf("shareable = %s", w.Body)
	}

	for body, want := range map[string]int{
		`{"name":"secret","path":"/data","owner":"theirs"}`:   http.StatusBadRequest,
		`{"name":"nope","path":"/data","owner":"ollama"}`:     http.StatusBadRequest,
		`{"name":"models","path":"/m","owner":"chat-feat"}`:   http.StatusBadRequest,
		`{"name":"models","path":"/models","owner":"ollama"}`: http.StatusCreated,
	} {
		if w := volumeCall(p, "founder", "POST", "app=chat", body); w.Code != want {
			t.Errorf("%s = %d %s, want %d", body, w.Code, w.Body, want)
		}
	}
	if w := volumeCall(p, "founder", "POST", "app=chat-feat", `{"name":"models","path":"/m","owner":"ollama"}`); w.Code != http.StatusBadRequest {
		t.Errorf("a preview borrowed a volume: %d", w.Code)
	}

	vols := listVolumes("chat")
	if len(vols) != 1 || vols[0].Owner != "ollama" || vols[0].DockerName("chat") != config.VolumeName("ollama", "models") {
		t.Fatalf("chat volumes = %+v", vols)
	}
	// A preview of the borrower gets an empty volume of its own, never the holder's files.
	preview, _ := getSource("chat-feat")
	if defs := p.volumeDefs(preview); len(defs) != 1 || defs[0].DockerName("chat-feat") != config.VolumeName("chat-feat", "models") {
		t.Fatalf("preview defs = %+v", defs)
	}

	if got := volumeBorrowers("ollama", "models"); len(got) != 1 || got[0] != "chat" {
		t.Fatalf("borrowers = %v", got)
	}
	if w := volumeCall(p, "founder", "DELETE", "app=ollama&name=models", ""); w.Code != http.StatusConflict {
		t.Errorf("deleting a volume in use = %d", w.Code)
	}
	if w := volumeCall(p, "founder", "DELETE", "app=chat&name=models", ""); w.Code != http.StatusNoContent {
		t.Errorf("stop using = %d %s", w.Code, w.Body)
	}
	if w := volumeCall(p, "founder", "DELETE", "app=ollama&name=models", ""); w.Code != http.StatusNoContent {
		t.Errorf("deleting a volume nobody else uses = %d", w.Code)
	}
}
