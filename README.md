# Skiff

**Ship it to a server you own.** Push-to-deploy with a web console, automatic HTTPS, managed databases, and preview environments — running on infrastructure you control, not rented. One small Go binary detects your stack, builds it, and runs it on Docker, local or over SSH.

```
$ skiff deploy

  Skiff v0.1.0

  Deploying myapp
  target   local docker
  build    Node.js

  ✓ Built skiff-myapp:latest
  ✓ Healthy
  ✓ Live at http://myapp.localhost:8080  (2.4s)
```

## Quickstart

You need Docker running.

```bash
git clone https://github.com/nuelScript/skiff
cd skiff
go build -o skiff .

# one terminal — the local router for *.localhost
./skiff proxy

# another — deploy the example, then open http://node-hello.localhost:8080
./skiff deploy -c examples/node-hello/skiff.toml
```

To deploy your own app: `cd` into it, run `skiff init`, then `skiff deploy`.

## What it builds

No Dockerfile required — Skiff detects the stack and generates the build:

| Stack | Detected by |
|---|---|
| **Node.js** | `package.json` — framework-aware (Next, Vite, Astro, SvelteKit, Remix, Nuxt, …) |
| **Python** | `requirements.txt` / `*.py` |
| **Go** | `go.mod` → multi-stage, tiny image |
| **Rust** | `Cargo.toml` → multi-stage |
| **Ruby** | `Gemfile` |
| **Elixir** | `mix.exs` → `mix release` |
| **Java** | Maven / Gradle |
| **.NET** | `*.csproj` |
| **PHP** | `index.php` |
| **Static** | `index.html` |

Have a `Dockerfile`? Skiff uses it instead. Need to tweak a step? Set `[build]`
commands in `skiff.toml` — the escape hatch short of a full Dockerfile.

## The platform

Run the web console with `skiff panel` (or set it up on a server) for the whole
platform in the browser:

- **Push-to-deploy** — connect GitHub and every push builds and ships, with
  zero-downtime rollout and instant rollback.
- **Preview environments** — every branch gets its own live URL and certificate.
- **Managed databases** — Postgres, MySQL, MongoDB, and Redis, provisioned and
  wired into your apps, with automatic daily backups.
- **Object storage** — S3-compatible buckets.
- **Custom domains + automatic HTTPS** — Let's Encrypt certificates, issued and
  renewed for you.
- **Autoscaling** — add and retire replicas to hold each app near a target CPU.
- **Workers & cron** — long-running background processes and scheduled jobs.
- **Alerts** — email, Slack, or webhook when a deploy fails, an app goes down, or
  5xx errors spike.
- **Teams, audit log, and API tokens** — collaborators, an activity trail, and a
  token-authenticated API for CI.

## Commands

| | |
|---|---|
| `skiff init` | scaffold a `skiff.toml` |
| `skiff deploy` | build + zero-downtime deploy |
| `skiff rollback` | instantly re-run a retained build, no rebuild |
| `skiff open <app>` | open a deployed app's URL |
| `skiff status` / `skiff ls` | app state + health / list apps |
| `skiff logs [-f] <app>` | app logs |
| `skiff down <app>` | stop + remove |
| `skiff sync` | prune orphans / dead entries |
| `skiff proxy` | local `*.localhost` router |
| `skiff server setup <user@host>` | bootstrap Docker on a fresh box over SSH |

The control plane itself — the web console (`skiff panel`) and edge router
(`skiff router`) — is normally run for you by the [server installer](#run-it-on-your-own-server).

## skiff.toml

```toml
name = "myapp"

[build]
port = 3000              # the port your app listens on
# dockerfile = "Dockerfile"

[env]                    # available at build + runtime
API_URL = "https://api.example.com"

[secrets]                # runtime only — never baked into the image
API_KEY = "..."

[resources]
memory = "512m"
cpu    = "0.5"

# [server]               # omit for local Docker
# host = "root@1.2.3.4"  # deploy to a remote box over SSH
```

A `.env` file next to `skiff.toml` is loaded too.

## Zero-downtime

Every deploy builds the new version, health-checks it, atomically cuts traffic over, then drains and retires the old one. If the new version never becomes healthy, it rolls back and the old one keeps serving.

## Run it on your own server

One command on a fresh Ubuntu/Debian box sets up the whole platform — Docker, the
edge router (`:80`/`:443` with automatic HTTPS), and the web console:

```bash
curl -fsSL https://useskiff.xyz/install | sh -s -- --domain example.com
```

Point `*.example.com` at the box, open `https://dash.example.com`, and log in with
the setup key it prints. The full walkthrough is in the
[self-hosting guide](https://useskiff.xyz/docs/self-hosting).

Prefer the CLI? `skiff server setup user@host` bootstraps Docker on a box so you
can `skiff deploy` to it over SSH.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) to get set up, and please follow our [Code of Conduct](CODE_OF_CONDUCT.md). Found a security issue? Report it privately per [SECURITY.md](SECURITY.md) — not in a public issue.

## License

Dual-licensed under either [MIT](LICENSE-MIT) or [Apache-2.0](LICENSE-APACHE), at your option.
