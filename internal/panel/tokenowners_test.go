package panel

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuelScript/skiff/internal/auth"
)

func TestOnlyOwnersRevokeOthersTokens(t *testing.T) {
	openTestDB(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".skiff"), 0o700)
	store := auth.NewStore(sqlDB)
	p := &Panel{auth: store}
	owner, team, err := store.CreateUser("o@acme.dev", "O", "password123")
	if err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUserNoTeam("m@acme.dev", "M", "password123")
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := store.CreateInvite("m@acme.dev", team.ID, auth.RoleMember)
	if _, err := store.AcceptInvite(inv.Token, member.ID); err != nil {
		t.Fatal(err)
	}
	_ = putSession("so", owner.ID, team.ID)
	_ = putSession("sm", member.ID, team.ID)
	ownerTok, _ := createToken(team.ID, "deploys", owner.ID, tokenGrant{})
	memberTok, _ := createToken(team.ID, "mine", member.ID, tokenGrant{})

	call := func(session, method, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/tokens"+query, strings.NewReader(`{}`))
		req.AddCookie(&http.Cookie{Name: "skiff_session", Value: session})
		w := httptest.NewRecorder()
		p.handleTokens(w, req)
		return w
	}
	if w := call("sm", http.MethodGet, ""); strings.Count(w.Body.String(), `"mine":true`) != 1 {
		t.Errorf("a member's listing marks only their own token: %s", w.Body)
	}
	if w := call("sm", http.MethodDelete, "?id="+ownerTok.ID); w.Code != http.StatusForbidden || len(listTokens(team.ID, "")) != 2 {
		t.Fatalf("a member revoked the owner's token: %d", w.Code)
	}
	if w := call("sm", http.MethodDelete, "?id="+memberTok.ID); w.Code != http.StatusNoContent {
		t.Fatalf("a member revoking their own token: %d %s", w.Code, w.Body)
	}
	other, _ := createToken(team.ID, "theirs", member.ID, tokenGrant{})
	if w := call("so", http.MethodDelete, "?id="+other.ID); w.Code != http.StatusNoContent || len(listTokens(team.ID, "")) != 1 {
		t.Fatalf("the owner revoking a member's token: %d %s", w.Code, w.Body)
	}
}
