package panel

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/nuelScript/skiff/internal/edge"
	"testing"
	"time"
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
