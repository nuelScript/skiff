package panel

import "testing"

func TestCloneURLAllowed(t *testing.T) {
	allowed := []string{
		"https://github.com/acme/api.git",
		"http://gitlab.internal/acme/api",
		"https://x-access-token:tok@github.com/acme/api.git", // token-injected form
	}
	for _, u := range allowed {
		if !cloneURLAllowed(u) {
			t.Errorf("%q should be allowed", u)
		}
	}

	// git's command-executing / non-http transports must be rejected.
	blocked := []string{
		`ext::sh -c "id"`,
		"ext::sh -c whoami",
		"file:///etc/passwd",
		"file::/tmp/x",
		"ssh://git@github.com/acme/api",
		"git@github.com:acme/api.git",
		"",
		"   ",
		"javascript:alert(1)",
	}
	for _, u := range blocked {
		if cloneURLAllowed(u) {
			t.Errorf("%q should be rejected", u)
		}
	}
}
