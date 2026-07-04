package builder

import "path/filepath"

type elixirBuilder struct{ dir string }

func (e *elixirBuilder) Name() string { return "Elixir" }

func (e *elixirBuilder) detect() bool {
	return fileExists(filepath.Join(e.dir, "mix.exs"))
}

func (e *elixirBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	return render(Plan{
		Base:    "elixir:1.16-slim",
		Install: []string{"mix local.hex --force && mix local.rebar --force && mix deps.get"},
		Build:   []string{"mix compile"},
		Env:     env,
		Start:   []string{"mix", "run", "--no-halt"},
		Port:    port,
	})
}
