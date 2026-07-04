package builder

import "path/filepath"

// staticBuilder serves a plain static site, detected by an index.html.
type staticBuilder struct{ dir string }

func (s *staticBuilder) Name() string { return "Static" }

func (s *staticBuilder) detect() bool {
	return fileExists(filepath.Join(s.dir, "index.html"))
}

func (s *staticBuilder) Dockerfile(port int) (string, error) {
	return render(Plan{StaticDir: ".", Port: port})
}
