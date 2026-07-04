package builder

import (
	"fmt"
	"strconv"
	"strings"
)

// Plan is a runtime-agnostic description of how to build and serve an app.
// A stack builder produces one; render turns it into a Dockerfile. This is the
// seam every language and framework flows through.
type Plan struct {
	Base       string   // build/runtime base image
	CacheFiles []string // manifest files to copy before Install (layer caching)
	Install    []string // install commands
	Build      []string // build commands (empty if none)

	// Exactly one of these says how to serve:
	StaticDir string   // serve this directory of static files, or
	Start     []string // run this command (rendered as an exec-form CMD)

	Port int
}

func render(p Plan) (string, error) {
	var b strings.Builder
	switch {
	case p.StaticDir != "":
		if needsBuildStage(p) {
			// Build in the runtime image, then serve the output from a tiny image.
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
		fmt.Fprintf(&b, "FROM %s\n", p.Base)
		writeAppBuild(&b, p)
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

// writeAppBuild writes the WORKDIR/COPY/install/build lines shared by the server
// stage and the static build stage. When CacheFiles are given, manifests are
// copied and installed before the source so the install layer caches across
// source-only changes.
func writeAppBuild(b *strings.Builder, p Plan) {
	b.WriteString("WORKDIR /app\n")
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

// execForm renders argv as a JSON array so CMD gets exec form (OS signals reach
// the app directly).
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
