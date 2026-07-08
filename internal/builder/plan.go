package builder

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Plan struct {
	Base       string
	CacheFiles []string
	Install    []string
	Build      []string
	Env        map[string]string

	// RuntimeBase, when set, makes the build multi-stage: the final image is RuntimeBase with Copy artifacts from the build stage.
	RuntimeBase string
	Copy        []Artifact

	// Exactly one of StaticDir or Start must be set.
	StaticDir string
	Start     []string

	Port int
}

type Artifact struct{ From, To string }

func render(p Plan) (string, error) {
	var b strings.Builder
	switch {
	case p.StaticDir != "":
		if needsBuildStage(p) {
			fmt.Fprintf(&b, "FROM %s AS build\n", p.Base)
			writeAppBuild(&b, p)
			b.WriteString("FROM busybox\nWORKDIR /site\n")
			fmt.Fprintf(&b, "COPY --from=build /app/%s/ ./\n", staticSrc(p.StaticDir))
		} else {
			b.WriteString("FROM busybox\nWORKDIR /site\n")
			fmt.Fprintf(&b, "COPY %s ./\n", cleanDir(p.StaticDir))
		}
		fmt.Fprintf(&b, "EXPOSE %d\n", p.Port)
		fmt.Fprintf(&b, "CMD [\"httpd\", \"-f\", \"-p\", \"%d\", \"-h\", \"/site\"]\n", p.Port)
	case len(p.Start) > 0:
		if p.RuntimeBase != "" {
			fmt.Fprintf(&b, "FROM %s AS build\n", p.Base)
			writeAppBuild(&b, p)
			fmt.Fprintf(&b, "FROM %s\n", p.RuntimeBase)
			b.WriteString("WORKDIR /app\n")
			for _, a := range p.Copy {
				fmt.Fprintf(&b, "COPY --from=build %s %s\n", a.From, a.To)
			}
		} else {
			fmt.Fprintf(&b, "FROM %s\n", p.Base)
			writeAppBuild(&b, p)
		}
		fmt.Fprintf(&b, "EXPOSE %d\n", p.Port)
		fmt.Fprintf(&b, "CMD %s\n", execForm(p.Start))
	default:
		return "", fmt.Errorf("invalid build plan: needs a static dir or a start command")
	}
	return b.String(), nil
}

func needsBuildStage(p Plan) bool {
	return len(p.Install) > 0 || len(p.Build) > 0
}

// writeAppBuild emits the shared build lines; with CacheFiles, manifests install before the source so the install layer caches across source-only changes.
func writeAppBuild(b *strings.Builder, p Plan) {
	b.WriteString("WORKDIR /app\n")
	writeEnv(b, p.Env)
	if len(p.CacheFiles) > 0 && len(p.Install) > 0 {
		fmt.Fprintf(b, "COPY %s ./\n", strings.Join(p.CacheFiles, " "))
		for _, c := range p.Install {
			fmt.Fprintf(b, "RUN %s\n", c)
		}
		b.WriteString("COPY . .\n")
	} else {
		b.WriteString("COPY . .\n")
		for _, c := range p.Install {
			fmt.Fprintf(b, "RUN %s\n", c)
		}
	}
	for _, c := range p.Build {
		fmt.Fprintf(b, "RUN %s\n", c)
	}
}

// writeEnv emits sorted ENV lines so the Dockerfile is deterministic (stable cache).
func writeEnv(b *strings.Builder, env map[string]string) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "ENV %s=%s\n", k, strconv.Quote(env[k]))
	}
}

// execForm renders argv as JSON so CMD is exec form and OS signals reach the app directly.
func execForm(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = strconv.Quote(a)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func cleanDir(d string) string {
	d = strings.TrimSuffix(d, "/")
	if d == "" || d == "." {
		return "."
	}
	return d
}

func staticSrc(d string) string {
	d = strings.TrimPrefix(d, "./")
	d = strings.TrimSuffix(d, "/")
	if d == "" {
		return "."
	}
	return d
}
