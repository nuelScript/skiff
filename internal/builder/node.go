package builder

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// nodeBuilder builds a Node.js app, detected by its package.json. It reads the
// dependencies to recognize a framework and pick the right Plan.
type nodeBuilder struct {
	dir      string
	fw       *nodeFramework
	resolved bool
}

func (n *nodeBuilder) Name() string {
	if fw := n.framework(); fw != nil {
		return fw.name
	}
	return "Node.js"
}

func (n *nodeBuilder) detect() bool {
	return fileExists(filepath.Join(n.dir, "package.json"))
}

func (n *nodeBuilder) Dockerfile(port int) (string, error) {
	plan := Plan{
		Base:    "node:20-slim",
		Install: []string{n.installCmd()},
		Port:    port,
	}
	switch fw := n.framework(); {
	case fw == nil:
		plan.Start = []string{"npm", "start"}
	case fw.staticDir != "":
		plan.Build = []string{"npm run build"}
		plan.StaticDir = fw.staticDir
	default:
		plan.Build = []string{"npm run build"}
		plan.Start = fw.serverCmd
	}
	return render(plan)
}

func (n *nodeBuilder) installCmd() string {
	switch {
	case fileExists(filepath.Join(n.dir, "pnpm-lock.yaml")):
		return "corepack enable && pnpm install --frozen-lockfile"
	case fileExists(filepath.Join(n.dir, "yarn.lock")):
		return "yarn install --frozen-lockfile"
	case fileExists(filepath.Join(n.dir, "package-lock.json")):
		return "npm ci"
	}
	return "npm install"
}

// framework returns the detected Node framework, or nil for a plain Node app.
func (n *nodeBuilder) framework() *nodeFramework {
	if !n.resolved {
		n.resolved = true
		deps := readDeps(filepath.Join(n.dir, "package.json"))
		for i := range nodeFrameworks {
			if _, ok := deps[nodeFrameworks[i].dep]; ok {
				n.fw = &nodeFrameworks[i]
				break
			}
		}
	}
	return n.fw
}

// nodeFramework maps a package.json dependency to how the app is built and served.
type nodeFramework struct {
	name      string
	dep       string   // dependency that signals this framework
	staticDir string   // static shape: `npm run build`, then serve this directory
	serverCmd []string // server shape: `npm run build`, then run this command
}

// Order matters: frameworks that also pull in `vite` (Astro, SvelteKit, Nuxt)
// must come before the plain Vite entry so the more specific one wins.
var nodeFrameworks = []nodeFramework{
	{name: "Next.js", dep: "next", serverCmd: []string{"npm", "start"}},
	{name: "Nuxt", dep: "nuxt", serverCmd: []string{"node", ".output/server/index.mjs"}},
	{name: "SvelteKit", dep: "@sveltejs/kit", serverCmd: []string{"node", "build"}},
	{name: "Remix", dep: "@remix-run/serve", serverCmd: []string{"npm", "start"}},
	{name: "TanStack Start", dep: "@tanstack/react-start", serverCmd: []string{"node", ".output/server/index.mjs"}},
	{name: "Astro", dep: "astro", staticDir: "dist"},
	{name: "Vite", dep: "vite", staticDir: "dist"},
	{name: "Create React App", dep: "react-scripts", staticDir: "build"},
	{name: "Vue CLI", dep: "@vue/cli-service", staticDir: "dist"},
}

func readDeps(path string) map[string]struct{} {
	deps := map[string]struct{}{}
	data, err := os.ReadFile(path)
	if err != nil {
		return deps
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return deps
	}
	for d := range pkg.Dependencies {
		deps[d] = struct{}{}
	}
	for d := range pkg.DevDependencies {
		deps[d] = struct{}{}
	}
	return deps
}
