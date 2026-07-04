# Skiff

**Effortless deploys on your own server.** One command, one small Go binary — build your app, run it, and get a live HTTPS URL. No cloud bill.

```
$ skiff deploy

  Skiff v0.1.0

  Deploying myapp
  target   root@203.0.113.10
  domain   myapp.you.dev
  build    Dockerfile

  ✓ Connected to root@203.0.113.10
  ✓ Built image from Dockerfile
  ✓ Started container
  ✓ Configured routes + TLS

  ✓ Live at https://myapp.you.dev  (2.4s)
```

## Why

Running your own apps shouldn't mean wrestling with servers. Skiff's bet is **developer experience**: deploys that feel effortless — one command, streaming logs, zero config — on a small box you own for a few dollars a month.

Built **in public**.

## Quickstart (dev)

```bash
git clone https://github.com/nuelScript/skiff
cd skiff
go run . --help

# try the deploy flow against the sample config
go run . deploy -c skiff.example.toml
```

## Config

Drop a `skiff.toml` in your app's repo (see [`skiff.example.toml`](skiff.example.toml)):

```toml
name   = "myapp"
domain = "myapp.you.dev"

[server]
host = "root@203.0.113.10"

[build]
dockerfile = "Dockerfile"
port       = 8080
```

## Roadmap

- One-command deploys to your own server
- Push-to-deploy
- Live logs
- Metrics + a dashboard
