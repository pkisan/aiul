# PROGRESS — AI Usage Logger

Single handoff file. Every new session reads CLAUDE.md then this file before doing anything.

Last updated: 2026-09-18

## Current phase

**Phase 0 — Foundations (macOS).**

Task in progress right now: Phase 0 complete except Docker Desktop, which is not
installed on this Mac. Waiting for the owner to verify the milestone.

## Plan for Phase 0

1. Create repo skeleton: /agent (Go module), /backend, /scripts, /docs, CLAUDE.md, .gitignore.
2. `git init` and first commit.
3. Check Homebrew tooling: go, php@8.3, composer, docker (Docker Desktop), mitmproxy. Report what is missing; the user installs or approves installs.
4. `go mod init` + `cmd/aiul` with a `version` command only.
5. docker-compose.yml with Postgres, Redis, MinIO (all bound to 127.0.0.1).
6. Write docs/DECISIONS.md and docs/SETUP-MAC.md.
7. Milestone verification commands handed to the user; STOP.

## Done

- Repo skeleton, `.gitignore` (blocks keys/certs/spool from day one), `CLAUDE.md`
- `docs/PROGRESS.md`, `docs/DECISIONS.md` (D1-D4), `docs/SETUP-MAC.md`
- Tooling check: Go 1.24.4, PHP 8.4.23 (Herd), Composer, mitmproxy 12.2.3, git present
- Go module `github.com/aayatti/aiul`; `cmd/aiul/main.go` with `aiul version`; `go vet` clean
- `docker-compose.yml`: Postgres 16 (127.0.0.1:5433), Redis 7 (127.0.0.1:6380), MinIO (127.0.0.1:9000/9001)
- git repository initialised, first commit `3e9c53c`

## In progress

- Nothing. Phase 0 is written; the owner verifies the milestone next.

## Next step

Owner runs the Phase 0 verification commands (see SETUP-MAC.md). Then STOP until
the owner confirms and asks for Phase 1.

## Left — Phase 0

- [x] Repo skeleton + CLAUDE.md + .gitignore + git init
- [x] Tooling check (go, php, composer, mitmproxy) — Docker Desktop MISSING
- [x] Go module + `aiul version`
- [x] docker-compose.yml (Postgres, Redis, MinIO)
- [x] docs/DECISIONS.md, docs/SETUP-MAC.md
- [ ] Owner installs Docker Desktop (`brew install --cask docker`) — needs approval, not run
- [ ] Milestone: `aiul version` runs; `docker compose ps` healthy; curl through mitmweb with explicit --proxy in one terminal only

## Left — later phases

- [ ] Phase 1 — dev root CA in Go (`aiul ca init|trust|untrust`), leaf minting, scripts/killswitch.sh FIRST
- [ ] Phase 2 — proxy engine (CONNECT, classify, mint, stream, SSE reassembly, parsers, spool)
- [ ] Phase 3 — redaction
- [ ] Phase 4 — endpoint agent (darwin platform impls, install/uninstall/status/doctor, forwarder)
- [ ] Phase 5 — task tagging (lsof → pid → cwd → git branch → task ID)
- [ ] Phase 6 — Laravel ingestion + storage + scoring
- [ ] Phase 7 — Inertia + Vue dashboard

## Blockers / open questions for the user

- **Docker Desktop is not installed.** It is only needed for the backend data
  services (Phase 6); Phases 1-5 do not need it. Install command shown in
  SETUP-MAC.md, not run without approval.
- PHP is 8.4.23 via Herd, not 8.3. Laravel 12 supports 8.4, so we use it (D4).
- Go module path is `github.com/aayatti/aiul` — say so if you want a different one,
  it is cheap to change now and annoying later.
- Before Phase 2 starts: the stdlib vs goproxy vs go-mitmproxy comparison (rule 11)
  must be written into DECISIONS.md and confirmed by the owner.

## Things the user must run by hand

- `brew install --cask docker` then `open -a Docker`, if and when Docker is wanted.
- The Phase 0 milestone verification commands.

## Machine state — settings currently changed on this Mac

**NONE.** No system proxy, no keychain trust, no env vars, no launchd jobs, no /etc/zshenv block.

Undo commands will be listed here the moment anything changes. From Phase 1 onward, `scripts/killswitch.sh` reverts everything in one command.
