package auth

import (
	"path/filepath"
	"testing"

	"github.com/nuelScript/skiff/internal/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := db.OpenAt(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("db.OpenAt: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewStore(database)
}

func TestCreateUserAndAuthenticate(t *testing.T) {
	s := newTestStore(t)
	u, team, err := s.CreateUser("dev@acme.dev", "Dev", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Email != "dev@acme.dev" || team.ID == "" {
		t.Fatalf("unexpected user/team: %+v / %+v", u, team)
	}

	if _, _, err := s.CreateUser("dev@acme.dev", "Dupe", "password123"); err == nil {
		t.Fatal("duplicate email should be rejected")
	}

	t.Run("correct password", func(t *testing.T) {
		got, ok := s.Authenticate("dev@acme.dev", "password123")
		if !ok || got.ID != u.ID {
			t.Fatalf("valid login rejected: ok=%v", ok)
		}
	})
	t.Run("wrong password", func(t *testing.T) {
		if _, ok := s.Authenticate("dev@acme.dev", "nope"); ok {
			t.Fatal("wrong password accepted")
		}
	})
	t.Run("unknown email", func(t *testing.T) {
		if _, ok := s.Authenticate("ghost@acme.dev", "password123"); ok {
			t.Fatal("unknown email accepted")
		}
	})
}

func TestChangePassword(t *testing.T) {
	s := newTestStore(t)
	u, _, err := s.CreateUser("dev@acme.dev", "Dev", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.ChangePassword(u.ID, "wrong-current", "newpassword123"); err == nil {
		t.Fatal("ChangePassword accepted a wrong current password")
	}
	if err := s.ChangePassword(u.ID, "password123", "newpassword123"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, ok := s.Authenticate("dev@acme.dev", "newpassword123"); !ok {
		t.Fatal("new password doesn't authenticate")
	}
	if _, ok := s.Authenticate("dev@acme.dev", "password123"); ok {
		t.Fatal("old password still works after change")
	}
}

func TestRoleMembership(t *testing.T) {
	s := newTestStore(t)
	u, team, err := s.CreateUser("owner@acme.dev", "Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if role, ok := s.Role(u.ID, team.ID); !ok || role != RoleOwner {
		t.Fatalf("creator role = %q ok=%v, want owner", role, ok)
	}

	other, _, err := s.CreateUser("other@acme.dev", "Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, ok := s.Role(other.ID, team.ID); ok {
		t.Fatal("a non-member reported a role in the team")
	}
}
