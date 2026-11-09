package panel

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/auth"
	"github.com/nuelScript/skiff/internal/edge"
)

func TestAccessTokensReachTheEdgeFile(t *testing.T) {
	openTestDB(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".skiff"), 0o700)
	_ = putSource(Source{App: "mcp", Team: "t1", Port: "3000"})
	_ = putSource(Source{App: "mcp-pr-1", Team: "t1", Port: "3000", Parent: "mcp"})
	_ = putSource(Source{App: "web", Team: "t1", Port: "3000"})
	tok, err := createAccessToken("mcp", "claude", time.Now())
	if err != nil || len(tok.Token) < 30 {
		t.Fatalf("%v %+v", err, tok)
	}
	if err := writeEdgeFile(); err != nil {
		t.Fatal(err)
	}
	rules := loadEdgeForTest()
	want := hashToken(tok.Token)
	if !slices.Contains(rules["mcp"].Tokens, want) || !slices.Contains(rules["mcp-pr-1"].Tokens, want) {
		t.Errorf("tokens at the edge: %+v", rules)
	}
	if _, ok := rules["web"]; ok {
		t.Errorf("an open app got a rule: %+v", rules["web"])
	}
	if list := listAccessTokens("mcp"); len(list) != 1 || list[0].Token != "" {
		t.Errorf("the list shows the token: %+v", list)
	}
}

func loadEdgeForTest() map[string]struct{ Tokens []string } {
	out := map[string]struct{ Tokens []string }{}
	for app, r := range edge.Load(edgeFile()) {
		out[app] = struct{ Tokens []string }{r.Tokens}
	}
	return out
}

// A model server installed behind a key has no login of its own: its last token can't be revoked, so it never ends up open.
func TestKeyedAppKeepsItsLastToken(t *testing.T) {
	openTestDB(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".skiff"), 0o700)
	store := auth.NewStore(sqlDB)
	p := &Panel{auth: store}
	user, team, err := store.CreateUser("a@acme.dev", "A", "password123")
	if err != nil {
		t.Fatal(err)
	}
	_ = putSession("s", user.ID, team.ID)
	_ = putSource(Source{App: "llm", Team: team.ID, Port: "11434", Catalog: "ollama"})
	first, _ := createAccessToken("llm", "installed with the app", time.Now())

	call := func(method, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/project/access-tokens?app=llm"+query, strings.NewReader(`{"name":"rotated"}`))
		req.AddCookie(&http.Cookie{Name: "skiff_session", Value: "s"})
		w := httptest.NewRecorder()
		p.handleAccessTokens(w, req)
		return w
	}
	if w := call(http.MethodGet, ""); !strings.Contains(w.Body.String(), `"required":true`) {
		t.Errorf("list: %s", w.Body)
	}
	if w := call(http.MethodDelete, "&id="+first.ID); w.Code != http.StatusBadRequest || len(listAccessTokens("llm")) != 1 {
		t.Fatalf("revoking the last token: %d %s", w.Code, w.Body)
	}
	if w := call(http.MethodPost, ""); w.Code != http.StatusOK {
		t.Fatalf("adding a token: %d %s", w.Code, w.Body)
	}
	if w := call(http.MethodDelete, "&id="+first.ID); w.Code != http.StatusNoContent {
		t.Fatalf("revoking once there's another: %d %s", w.Code, w.Body)
	}
}
