# SETUP-MAC

How to get a Mac ready to work on this repo. Everything here is repo-local:
**nothing in Phase 0 changes a system setting.** No proxy, no keychain trust, no
environment variables, no launchd jobs.

## What must be installed

| Tool | Why | Check |
| --- | --- | --- |
| Go 1.22+ | builds the `aiul` binary | `go version` |
| PHP 8.3+ | the Laravel backend (Phase 6) | `php -v` |
| Composer | PHP dependencies | `composer --version` |
| Docker Desktop | local Postgres, Redis, MinIO | `docker --version` |
| mitmproxy | **research tool only**, run by hand | `mitmproxy --version` |
| git | version control | `git --version` |

Status on this machine as of 2026-09-18: Go 1.24.4, PHP 8.4.23 (Herd),
Composer, mitmproxy 12.2.3 and git are present. **Docker Desktop is not installed.**

### Installing Docker Desktop

```sh
brew install --cask docker
open -a Docker      # first launch; it asks for your password to install its helper
```

This is a large install and it adds a background helper, so run it when convenient.
It is only needed for the backend data services — the Go work in Phases 1–5 does
not need Docker at all.

## Building the binary

```sh
cd agent
go build -o aiul ./cmd/aiul
./aiul version
```

`agent/aiul` is git-ignored.

## Local data services

```sh
docker compose up -d
docker compose ps        # all three should say healthy
docker compose down      # stop them
docker compose down -v   # stop and delete the data volumes
```

Ports are bound to `127.0.0.1` only, so nothing is reachable from your network:

- Postgres `127.0.0.1:5433` (db `aiul`, user `aiul`, password `aiul_dev_password`)
- Redis `127.0.0.1:6380`
- MinIO S3 API `127.0.0.1:9000`, web console `http://127.0.0.1:9001` (user `aiul`, password `aiul_dev_password`)

Postgres and Redis use non-default host ports on purpose, so they cannot collide
with anything Herd or another project already runs.

## Using mitmproxy as a research tool

mitmproxy is a ready-made intercepting proxy. We use it only to *look* at what an
AI tool sends, so we can write a Go parser for that format. We never configure it
machine-wide and the product never depends on it.

Terminal 1 — start it:

```sh
mitmweb --listen-host 127.0.0.1 --listen-port 8080
```

That opens a web view of the captured traffic in your browser. On first run it
writes a CA certificate to `~/.mitmproxy/mitmproxy-ca-cert.pem`.

Terminal 2 — send exactly one command through it:

```sh
curl -v --proxy http://127.0.0.1:8080 \
     --cacert ~/.mitmproxy/mitmproxy-ca-cert.pem \
     https://example.com
```

`--proxy` and `--cacert` apply to that one `curl` only. Nothing else on the Mac is
affected, and closing Terminal 1 ends the interception completely.

In the output you should see `issuer: CN=mitmproxy` — proof the connection was
decrypted and re-encrypted by the proxy rather than coming straight from the site.
Without `--cacert` the same command fails with a certificate error, which is the
correct, safe behaviour.

## Safety rules while working in this repo

- Nothing is set machine-wide without the exact commands being shown first and an
  explicit "yes".
- From Phase 1 onward, keep a second terminal open with `scripts/killswitch.sh`
  ready. One command undoes every system change this project makes.
- `docs/PROGRESS.md` always lists every system setting currently changed, with the
  exact command to undo each one. Today that list is empty.
