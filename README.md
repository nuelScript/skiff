# Skiff

**Effortless deploys on your own server.** One small Go binary: point it at an app, it detects the stack, builds it, and runs it — on local Docker or a remote box over SSH. No cloud bill.

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

No Dockerfile required — Skiff detects the stack:

| Stack | Detected by | Served |
|---|---|---|
| **Node.js** | `package.json` | `npm start`; framework-aware (Next, Vite, Astro, SvelteKit, Remix, Nuxt, …) → build + run |
| **Python** | `requirements.txt` / `*.py` | entrypoint or a `Procfile` |
| **Go** | `go.mod` | multi-stage → tiny image |
| **PHP** | `index.php` | built-in server |
| **Static** | `index.html` | tiny static server |

Have a `Dockerfile`? Skiff uses it instead — the escape hatch.

## Commands

| | |
|---|---|
| `skiff init` | scaffold a `skiff.toml` |
| `skiff deploy` | build + zero-downtime deploy |
| `skiff proxy` | local `*.localhost` router |
| `skiff status` | container state + health |
| `skiff ls` | list apps |
| `skiff logs [-f]` | app logs |
| `skiff down <app>` | stop + remove |
| `skiff sync` | prune orphans / dead entries |

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
