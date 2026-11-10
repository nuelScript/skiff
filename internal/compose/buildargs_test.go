package compose

import (
	"strings"
	"testing"
)

func TestSecretBuildArgsAreNoted(t *testing.T) {
	f, err := Parse([]byte(`services:
  web:
    build:
      context: .
      args: {NPM_TOKEN: abc, NODE_ENV: production}
`), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	notes := strings.Join(BuildPlan("shop", f, noFiles, func(string) PathKind { return PathDir }).Notes, "\n")
	if !strings.Contains(notes, "build arg NPM_TOKEN stays readable") || strings.Contains(notes, "NODE_ENV") || strings.Contains(notes, "abc") {
		t.Errorf("notes:\n%s", notes)
	}
}
