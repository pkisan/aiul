# CLAUDE.md — AI Usage Logger (aiul)

## Read this first, every session

**Before doing anything else, read `docs/PROGRESS.md`.** It is the single handoff
file: current phase, the task in progress, what is done (with commit hashes), the
next concrete step, and every system setting currently changed on this Mac.
Then check `git log --oneline -10` and `git status` match it. If the last task was
interrupted mid-way, say what state it is in before continuing.

Update `docs/PROGRESS.md` after every meaningful step (each file, test or command
finished) — not just at the end of a phase — and commit it with the work.

## What this is

A module of the owner's PM tool that captures AI tool usage on managed company
devices, attributes each interaction to a task, redacts secrets, scores prompt
quality and reports usage. Source of truth for architecture and phases:
`AI-Usage-Logger-Build-Map.md`, with two deliberate changes from that map:

1. **macOS first**, not Linux. Linux and Windows come later. Every OS-specific
   piece sits behind a Go interface in `internal/platform` with a darwin
   implementation now and stubs for linux and windows.
2. **Go from the start**, not Python. The agent and the TLS-inspecting proxy are
   ONE Go binary, `aiul`. There is no Python in this repo and no later rewrite.
   mitmproxy is a *research tool only*: the owner runs `mitmweb` in a separate
   terminal to watch what a tool sends, so we can write a parser for it. The
   product never depends on it.

## Who the owner is

A PHP/Laravel and WordPress developer, new to Go, TLS, certificates and macOS
system administration. Explain each new concept in one or two plain sentences the
first time it appears, and comment non-obvious code. Prefer clear, boring Go over
clever Go.

## Repo layout

```
/agent    Go module, single `aiul` binary
          cmd/aiul            entry point and CLI commands
          internal/ca         dev CA creation, leaf minting, cert cache
          internal/proxy      explicit HTTPS proxy, host classification,
                              streaming, SSE reassembly
          internal/parsers    one parser per provider/tool behind an interface
          internal/redact     redaction rules
          internal/platform   OS interfaces; darwin first, linux/windows stubs
          internal/forward    local event spool and backend forwarder
          testdata/           recorded, anonymised traffic fixtures
/backend  PHP 8.3+, Laravel 12, Horizon, Inertia + Vue
/scripts  helper shell scripts, including the kill switch
/docs     DECISIONS.md, SETUP-MAC.md, PROGRESS.md — all kept up to date
```

## Non-negotiable rules

1. **Safety of the owner's machine.** This project intercepts the same network the
   session runs through. Never set machine-wide proxy settings, environment
   variables or keychain trust without first (a) showing the exact commands,
   (b) confirming the kill switch works, and (c) getting an explicit "yes". Test
   first with per-command env vars in a separate terminal.
2. **Kill switch first.** Before any system change, `scripts/killswitch.sh` must
   exist and work: removes the system proxy on every network service, unsets
   launchctl env vars, unloads our launchd jobs, removes our `/etc/zshenv` block.
   Idempotent, one command.
3. **Only intercept AI domains.** Certificates are minted and traffic decrypted
   ONLY for hostnames on a versioned allow-list of exact AI hostnames. Patterns
   are anchored to whole hostnames (never match `notopenai.com.evil.net`). Never
   allow-list broad shared domains (all of google.com, googleapis.com, github.com).
   Everything else passes through sealed: no cert minted, nothing logged. The
   matcher has its own unit tests.
4. **Trust, do not break.** If a client rejects our certificate during the
   handshake, add that host to a tunnel list and from then on pass it through
   sealed, logging metadata only (host, timing, bytes). Never break a tool.
5. **Never weaken upstream checks.** Our own connection to the real provider always
   verifies the provider's certificate against the normal system roots. No
   `InsecureSkipVerify` anywhere, including tests against real hosts.
6. **Stream without delay.** Responses are forwarded chunk by chunk and flushed
   immediately (`http.Flusher`). Logging copies chunks on the side. The user never
   waits for a full response.
7. **Fail open.** If the proxy is unhealthy, the agent removes proxy settings
   rather than blocking traffic.
8. **Redact before storage.** Secrets and obvious PII are masked on our copy before
   anything is written or sent. The request forwarded to the provider is never
   modified. Raw secrets never persist.
9. **Dev CA only.** `aiul ca init` generates a dev root CA with our own Go code,
   stored under `~/Library/Application Support/AIUL/dev-ca/`, private key 0600.
   Never commit keys or certificates. The production chain (per-tenant root in
   KMS/HSM signing a short-lived, name-constrained intermediate per device) is
   designed in `docs/DECISIONS.md` but not built.
10. **MDM gate.** The agent checks enrollment with `profiles status -type enrollment`.
    Dev override `AIUL_DEV_ALLOW_UNMANAGED=1` logs a loud warning.
11. **Dependencies.** Prefer the Go standard library (`net/http`, `crypto/tls`,
    `crypto/x509`). Before Phase 2, compare stdlib-only vs goproxy vs go-mitmproxy,
    recommend one with reasons, record it in `docs/DECISIONS.md`, and wait for
    confirmation.
12. **No signing, notarization or packaging yet.** That is Phase 8.
13. **One phase at a time.** At the end of each phase: list files changed, give the
    exact commands the owner runs to verify the milestone, and STOP until they
    confirm. Never start the next phase unprompted.
14. **Assume the session can end at any moment.** `docs/PROGRESS.md` is the handoff.
    Write the plan into it before starting anything long. Small, frequent commits
    with clear messages.

## Research workflow (new tool or web app)

The owner runs only that tool through `mitmweb` in a separate terminal and captures
a few real interactions. We anonymise them into `agent/testdata/` fixtures, then
write the Go parser against those fixtures.

## Phases (macOS)

0 Foundations · 1 Dev root CA in Go · 2 Proxy engine · 3 Redaction ·
4 Endpoint agent · 5 Task tagging · 6 Laravel ingestion + storage ·
7 Minimal dashboard · 8 Signing and packaging.

Full phase detail lives in the original prompt summary in `docs/PROGRESS.md`
("Left — later phases") and in `AI-Usage-Logger-Build-Map.md`.
