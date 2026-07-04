package builder

import "fmt"

type customBuilder struct {
	base, install, build, start, static string
}

// Custom builds an app from an explicit recipe in skiff.toml instead of
// auto-detection — the escape hatch short of a full Dockerfile.
func Custom(base, install, build, start, static string) Builder {
	return &customBuilder{base, install, build, start, static}
}

func (c *customBuilder) Name() string { return "custom" }

func (c *customBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	p := Plan{Base: c.base, Env: env, Port: port}
	if c.install != "" {
		p.Install = []string{c.install}
	}
	if c.build != "" {
		p.Build = []string{c.build}
	}
	switch {
	case c.static != "":
		p.StaticDir = c.static
		if (len(p.Install) > 0 || len(p.Build) > 0) && c.base == "" {
			return "", fmt.Errorf("a custom static build needs [build] base")
		}
	case c.start != "":
		if c.base == "" {
			return "", fmt.Errorf("a custom build needs [build] base")
		}
		p.Start = []string{"sh", "-c", c.start}
	default:
		return "", fmt.Errorf("a custom build needs [build] start or [build] static")
	}
	return render(p)
}
