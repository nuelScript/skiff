package builder

import (
	"os"
	"strings"
)

// dotnetBuilder is best-effort and unverified locally; ASP.NET binds via ASPNETCORE_URLS set from $PORT at start.
type dotnetBuilder struct{ dir string }

func (d *dotnetBuilder) Name() string { return ".NET" }

func (d *dotnetBuilder) detect() bool {
	return hasFileWithExt(d.dir, ".csproj") || hasFileWithExt(d.dir, ".sln")
}

func (d *dotnetBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	name := csprojName(d.dir)
	return render(Plan{
		Base:        "mcr.microsoft.com/dotnet/sdk:8.0",
		Build:       []string{"dotnet publish -c Release -o /app/out"},
		Env:         env,
		RuntimeBase: "mcr.microsoft.com/dotnet/aspnet:8.0",
		Copy:        []Artifact{{From: "/app/out", To: "/app"}},
		Start:       []string{"sh", "-c", "ASPNETCORE_URLS=http://+:$PORT dotnet /app/" + name + ".dll"},
		Port:        port,
	})
}

func csprojName(dir string) string {
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".csproj") {
				return strings.TrimSuffix(e.Name(), ".csproj")
			}
		}
	}
	return "app"
}

func hasFileWithExt(dir, ext string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			return true
		}
	}
	return false
}
