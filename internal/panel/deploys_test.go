package panel

import "testing"

func TestInjectToken(t *testing.T) {
	if got := injectToken("https://github.com/acme/api.git", "tok"); got != "https://tok@github.com/acme/api.git" {
		t.Fatalf("https inject = %q", got)
	}
	// Non-https URLs pass through untouched — the token must never be spliced into
	// an unexpected scheme.
	for _, u := range []string{"http://x/y", "git@github.com:acme/api.git", "ssh://git@host/repo"} {
		if got := injectToken(u, "tok"); got != u {
			t.Fatalf("injectToken(%q) modified a non-https URL: %q", u, got)
		}
	}
}

func TestBeginBuildSupersede(t *testing.T) {
	_, superseded1, done1 := beginBuild("supersede-app", "id1")
	defer done1()
	if superseded1() {
		t.Fatal("first build reported superseded before any newer build")
	}

	// A newer build for the same app supersedes the first.
	_, superseded2, done2 := beginBuild("supersede-app", "id2")
	defer done2()
	if !superseded1() {
		t.Fatal("first build not superseded by a newer one")
	}
	if superseded2() {
		t.Fatal("second (current) build wrongly reported superseded")
	}

	// Cleaning up the first build must not evict the second's in-flight entry.
	done1()
	if superseded2() {
		t.Fatal("cleanup of the first build evicted the current one")
	}
}

func TestCancelInflight(t *testing.T) {
	_, superseded, done := beginBuild("cancel-app", "idA")
	defer done()

	if cancelInflight("cancel-app", "idB") {
		t.Fatal("cancelInflight matched a non-current id")
	}
	if !cancelInflight("cancel-app", "idA") {
		t.Fatal("cancelInflight didn't match the current id")
	}
	if !superseded() {
		t.Fatal("a canceled build should read as superseded (ctx canceled)")
	}
}
