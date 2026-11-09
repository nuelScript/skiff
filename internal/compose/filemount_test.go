package compose

import (
	"strings"
	"testing"
)

// A file mounted writable from the repository is built into the image, so the plan says the app's changes to it don't last.
func TestWritableFileMountsSayTheyReset(t *testing.T) {
	f, err := Parse([]byte(`services:
  web:
    image: nginx
    volumes: ["./app.db:/data/app.db", "./nginx.conf:/etc/nginx/nginx.conf:ro"]
`), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	notes := strings.Join(BuildPlan("shop", f, noFiles, func(string) PathKind { return PathFile }).Notes, "\n")
	if !strings.Contains(notes, "./app.db is built into its image from the repository, so changes the app makes to it are lost on each deploy") {
		t.Errorf("no warning for the writable file:\n%s", notes)
	}
	if strings.Contains(notes, "./nginx.conf is built into its image from the repository, so changes the app makes") {
		t.Errorf("a read-only file was warned about:\n%s", notes)
	}
}
