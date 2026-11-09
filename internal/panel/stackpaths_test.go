package panel

import "testing"

func TestRepoRelCatchesPathsOutsideTheRepository(t *testing.T) {
	cases := []struct {
		dir, p, want string
		ok           bool
	}{
		{"/", ".", ".", true},
		{"/", "./api", "api", true},
		{"/deploy", "../api", "api", true},
		{"/deploy", "..", ".", true},
		{"/", "..", "..", false},
		{"/deploy", "../..", "..", false},
		{"/a/b", "../../../etc", "../etc", false},
		{"/", "..foo", "..foo", true},
	}
	for _, c := range cases {
		got, ok := repoRel(c.dir, c.p)
		if got != c.want || ok != c.ok {
			t.Errorf("repoRel(%q, %q) = %q %v, want %q %v", c.dir, c.p, got, ok, c.want, c.ok)
		}
	}
}
