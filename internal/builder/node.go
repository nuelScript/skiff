package builder

import "path/filepath"

// nodeBuilder builds a Node.js app, detected by its package.json.
type nodeBuilder struct{ dir string }

func (n *nodeBuilder) Name() string { return "Node.js" }

func (n *nodeBuilder) detect() bool {
	return fileExists(filepath.Join(n.dir, "package.json"))
}

func (n *nodeBuilder) Dockerfile(port int) (string, error) {
	// Choose the install command from whichever lockfile is present.
	install := "npm install"
	switch {
	case fileExists(filepath.Join(n.dir, "pnpm-lock.yaml")):
		install = "corepack enable && pnpm install --frozen-lockfile"
	case fileExists(filepath.Join(n.dir, "yarn.lock")):
		install = "yarn install --frozen-lockfile"
	case fileExists(filepath.Join(n.dir, "package-lock.json")):
		install = "npm ci"
	}
	return render(Plan{
		Base:    "node:20-alpine",
		Install: []string{install},
		Start:   []string{"npm", "start"},
		Port:    port,
	})
}
