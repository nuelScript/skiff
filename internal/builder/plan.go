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
	Base    string   // build/runtime base image (server apps)
	Install []string // install commands
	Build   []string // build commands (empty if none)

	// Exactly one of these says how to serve:
	StaticDir string   // serve this directory of static files, or
	Start     []string // run this command (rendered as an exec-form CMD)

	Port int
}

func render(p Plan) (string, error) {
	var b strings.Builder
	switch {
	case p.StaticDir != "":
		// Serve pre-built static files from a tiny image.
		b.WriteString("FROM busybox\n")
		b.WriteString("WORKDIR /site\n")
		fmt.Fprintf(&b, "COPY %s ./\n", cleanDir(p.StaticDir))
		fmt.Fprintf(&b, "EXPOSE %d\n", p.Port)
		fmt.Fprintf(&b, "CMD [\"httpd\", \"-f\", \"-p\", \"%d\", \"-h\", \"/site\"]\n", p.Port)
	case len(p.Start) > 0:
		fmt.Fprintf(&b, "FROM %s\n", p.Base)
		b.WriteString("WORKDIR /app\n")
		b.WriteString("COPY . .\n")
		for _, c := range p.Install {
			fmt.Fprintf(&b, "RUN %s\n", c)
		}
		for _, c := range p.Build {
			fmt.Fprintf(&b, "RUN %s\n", c)
		}
		fmt.Fprintf(&b, "EXPOSE %d\n", p.Port)
		fmt.Fprintf(&b, "CMD %s\n", execForm(p.Start))
	default:
		return "", fmt.Errorf("invalid build plan: needs a static dir or a start command")
	}
	return b.String(), nil
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
