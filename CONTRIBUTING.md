# Contributing to Skiff

Thanks for your interest in Skiff. This guide covers how to get set up, the
conventions we follow, and how to get a change merged.

By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).
Found a security issue? Please **don't open a public issue** — see
[SECURITY.md](SECURITY.md).

## Ways to help

- **Report a bug** or **request a feature** via the issue templates.
- **Improve the docs** — even a typo fix is welcome.
- **Add a stack, command, or fix** — see the workflow below.

For anything larger than a small fix, please open an issue first so we can agree
on the approach before you spend time on it.

## Prerequisites

- **Go** — the version pinned in [`go.mod`](go.mod) (the `toolchain` line; Go
  will fetch it automatically).
- **Docker**, running — Skiff builds and runs apps through it.
- **Node.js 22+** — only if you're changing the web dashboard.

## Getting set up

```bash
git clone https://github.com/nuelScript/skiff
cd skiff
go build -o skiff .

# run the local *.localhost router in one terminal…
./skiff proxy
# …and deploy an example in another
./skiff deploy -c examples/node-hello/skiff.toml
```

## Project layout

| Path | What lives here |
|---|---|
| `cmd/` | the CLI commands (one file per command, wired up in `root.go`) |
| `internal/builder/` | stack detection + Dockerfile generation (one file per stack) |
| `internal/docker/` | the Docker engine wrapper (local or remote over SSH) |
| `internal/router/`, `internal/proxy/` | the edge router and local dev proxy |
| `internal/panel/` | the hosted control panel (HTTP API + background loops) |
| `internal/db/`, `internal/auth/`, `internal/config/` | storage, accounts, config |
| `web/dash/` | the dashboard (Vite + React + TypeScript) |
| `examples/` | sample apps used to test deploys |

A couple of common changes:

- **New stack** → add one file to `internal/builder/` with detection + a `Plan`,
  register it, and add an `examples/<name>/` app to test it.
- **New CLI command** → add `cmd/<name>.go` and register it in `cmd/root.go`.

## Development workflow

The [`Makefile`](Makefile) has the common tasks:

```bash
make test        # go test ./...
make vet         # go vet ./...
make fmt         # gofmt -w .
make fmt-check   # fail if anything isn't gofmt-clean
make lint        # golangci-lint run (see .golangci.yml)
make build       # build the CLI
```

Please make sure `make fmt-check`, `make vet`, and `make test` all pass before
opening a PR — CI runs the same checks.

### Changing the dashboard

The dashboard is embedded into the Go binary from `internal/panel/dist`, which
is **committed to the repo**. If you change anything under `web/dash/`, rebuild
and sync the embedded copy, and commit the result:

```bash
make dash        # builds web/dash and copies it into internal/panel/dist
```

### Tests

Prefer table-driven tests with `t.Run` subtests, and keep them dependency-free
(the suite uses the standard `testing` package). Behavior that needs a live
Docker daemon is verified manually against real Docker rather than in committed
tests — keep committed tests to pure logic and anything backed by a temp SQLite
database (`db.OpenAt(t.TempDir())`).

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/): a
`type(scope): summary` subject line, lowercase, no trailing period.

```
fix(router): cache discovered routes so requests don't fork docker each time
feat(builder): detect Bun projects
docs: clarify the skiff.toml secrets section
```

Common **types**: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`,
`chore`. **Scope** is the package or area (`panel`, `docker`, `router`, `dash`,
`db`, `cli`, …) and is optional. Keep each commit focused; write the body to
explain *why* when it isn't obvious.

## Pull requests

1. Fork the repo and create a branch off `main` (`fix/…`, `feat/…`).
2. Make your change, with tests where it makes sense.
3. Run `make fmt-check vet test` (and `make dash` if you touched the dashboard).
4. Open the PR against `main`, fill in the template, and link any related issue.
5. Keep the PR focused — smaller PRs get reviewed faster. Rebase on `main` if it
   drifts; we prefer a clean, linear history.

Please don't include unrelated formatting churn or secrets/credentials in a
diff. Maintainers may squash-merge.

## Reporting security issues

Never file a public issue for a vulnerability. Follow the private disclosure
process in [SECURITY.md](SECURITY.md).

## License

Skiff is dual-licensed under either [MIT](LICENSE-MIT) or
[Apache-2.0](LICENSE-APACHE), at your option. Unless you state otherwise, any
contribution you intentionally submit for inclusion in the project is
dual-licensed as above, without any additional terms or conditions.
