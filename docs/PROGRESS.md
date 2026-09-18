# PROGRESS — AI Usage Logger

Single handoff file. Every new session reads CLAUDE.md then this file before doing anything.

Last updated: 2026-09-18

## Current phase

**Phase 4 — Endpoint agent (darwin first).**

Task in progress right now: `internal/platform` interfaces and their darwin
implementations.

Phase 3 is code-complete (63 tests, -race clean). The owner has NOT yet reported
the by-hand milestone result; ask before assuming it passed.

## Plan for Phase 4

Nothing in this phase touches the Mac without showing the exact commands and
getting an explicit yes. `aiul install` defaults to a dry run.

1. `internal/platform`: interfaces for ProxyConfigurator, EnvWriter, MDMChecker,
   ToolDetector, ServiceManager, plus the existing TrustInstaller.
2. darwin implementations:
   - proxy via `networksetup` on every active network service
   - terminal env vars via a marked block in `/etc/zshenv`
   - GUI env vars via a LaunchAgent running `launchctl setenv` at login
   - LaunchDaemon plist for `aiul run`, LaunchAgent plist for per-user setup
   - MDM check via `profiles status -type enrollment`, with
     AIUL_DEV_ALLOW_UNMANAGED=1 as a loud dev override
   - detect claude, codex, gemini, opencode, Cursor, VS Code
3. linux/windows stubs for each, so the module keeps building for every GOOS.
4. `internal/forward`: forwarder reading the spool, batching to the backend with a
   device token from the keychain, backoff, delete only after confirmation, its own
   connection bypassing the proxy.
5. `aiul run` (proxy + agent loop), `aiul install`, `aiul uninstall`, `aiul status`,
   `aiul doctor`.
6. Health checks: re-apply drifted settings, and fail open — remove the proxy
   setting if the proxy is unhealthy.
7. Record the privilege split in DECISIONS.md.

Phase 2 is DONE, milestone passed with Claude Code. The owner decided:
spool only parsed conversations (done, `TestHousekeepingCallsAreNotStored`);
leave brotli/zstd undecoded for now; start Phase 3.

## Plan for Phase 3

1. `internal/redact/redact.go` — one rule list, each rule a name plus a compiled
   regexp plus how to mask it. Text-like bodies only.
2. Rules: OpenAI, Anthropic, AWS, GitHub, Stripe, Google keys; bearer tokens;
   password/secret/token in key=value and JSON form; PEM private key blocks;
   emails; phone numbers; Aadhaar; PAN.
3. `redact_test.go` — one test per rule, plus tests that non-secrets are left
   alone (no over-masking) and that masking is stable.
4. Wire it into `proxy.record` so every Event is redacted before it reaches the
   sink. The request forwarded to the provider is never touched.
5. A proxy test proving a fake API key in a prompt is masked in the event while
   the provider received the original bytes.

Phase 1 is DONE: the owner confirmed the Safari test passed (no warning while
trusted, warning again after untrust).

## Plan for Phase 2

Written before starting, so an interruption loses nothing. In order:

1. `internal/proxy/hosts.go` + tests — versioned allow-list of exact AI hostnames,
   anchored matcher, tunnel list. Nothing else may decide what gets decrypted.
2. `internal/proxy/certs.go` + tests — bounded in-memory leaf cache keyed by host,
   respecting expiry.
3. `internal/proxy/proxy.go` — listener on 127.0.0.1:8899, CONNECT handling,
   classify, and raw pass-through for pass/tunnel. Milestone-able on its own.
4. Capture path — dial upstream with full verification, copy the real certificate's
   SAN names, mint, serve via `tls.Config.GetCertificate`, ALPN http/1.1.
5. Streaming — forward chunk by chunk with `http.Flusher`, tee a copy for logging.
   Test proves a chunk arrives before the response ends.
6. Handshake-failure detection feeding the tunnel list (rule 4).
7. Decompression (gzip/br/zstd) on our copy only; SSE reassembly.
8. `internal/parsers` — interface + OpenAI, Anthropic, Gemini against fixtures.
9. `internal/forward` spool — one JSON event per interaction.
10. `aiul proxy` command, then the milestone run.

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
- Go module `github.com/pkisan/aiul`; `cmd/aiul/main.go` with `aiul version`; `go vet` clean
- `docker-compose.yml`: Postgres 16 (127.0.0.1:5433), Redis 7 (127.0.0.1:6380), MinIO (127.0.0.1:9000/9001)
- git repository initialised, first commit `3e9c53c`

### Phase 1 (in progress)

- `scripts/killswitch.sh` — written, dry run verified clean on this Mac (`7ff6d87`)
- Go module renamed to `github.com/pkisan/aiul` (`7ff6d87`)
- `internal/ca/ca.go` — dev root creation, load, paths, fingerprint; key written 0600
- `internal/ca/leaf.go` — `MintLeaf`, 24h leaves, SAN from hosts, ECDSA P-256
- `internal/ca/ca_test.go` — 7 tests, all passing, including a real TLS handshake
  against a server using a leaf we minted

- `internal/platform`: `TrustInstaller` interface, `trust_darwin.go` (security
  add-trusted-cert / remove-trusted-cert), linux and windows stubs returning
  `ErrUnsupported`. Verified the module builds for all three GOOS values.
- `cmd/aiul/ca.go`: `ca init|info|trust|untrust|demo-server`. trust and untrust
  print the exact commands and require an explicit yes.
- Dev CA created on this Mac: `AIUL Dev Root - VWS18s-MacBook-Air.local`,
  SHA-256 `1D E5 55 8B ...`, key mode confirmed `-rw-------`.
- Smoke test passed: curl against `aiul ca demo-server` fails without our CA
  (`SSL certificate problem: self signed certificate in certificate chain`) and
  succeeds with `--cacert root.crt`. **No keychain change was made.**

## In progress

- Nothing.

## Next step

Write the `internal/platform` interfaces, then `proxyconf_darwin.go`.

## Left — Phase 1

- [x] scripts/killswitch.sh (BEFORE any system change) + dry run verified
- [x] internal/ca: root creation, load, leaf minting
- [x] internal/ca unit tests
- [x] internal/platform TrustInstaller + darwin impl + linux/windows stubs
- [x] `aiul ca init|info|trust|untrust` commands (trust shows the command and asks first)
- [x] `aiul ca demo-server` — tiny local HTTPS server on a minted leaf, for the Safari test
- [x] Milestone CONFIRMED by the owner: Safari showed no warning while trusted and
      warned again after untrust

## Left — Phase 2

- [x] D5 recorded: standard library only, no proxy framework (`5cc3022`)
- [x] hosts.go: allow-list v1 + anchored matcher + tunnel list, 6 tests passing with -race
- [x] certs.go: bounded LRU leaf cache, renews within 1h of expiry, 6 tests with -race
- [x] proxy.go: CONNECT, classify, pass/tunnel raw pass-through, hijack + rewind
- [x] capture path: verified upstream dial (`UpstreamRootCAs`, nil = system roots,
      no InsecureSkipVerify anywhere), SAN names copied from the real certificate,
      minted leaf, ALPN http/1.1, warns if the client wants another protocol
- [x] streaming with immediate flush + test proving a chunk arrives while the
      response is still open. Fixed a real bug found by that test: the chunked
      terminator was missing, so clients hung until their own timeout. Regression
      test added.
- [x] handshake-failure detection feeds the tunnel list; test proves the tool
      works again on the retry
- [x] gzip/deflate decompression on our copy; br and zstd are reported unreadable
      rather than stored as rubbish (see the open question below); SSE reassembly
- [x] parsers: interface + OpenAI, Anthropic, Gemini against anonymised fixtures in
      `agent/testdata/`, including automated-follow-up detection (tool results)
- [x] forward: JSON event spool, one file per event, 0600, atomic rename
- [x] `aiul proxy` command
- [x] Verified live from this session: `curl https://example.com` through the proxy
      showed its REAL issuer (Cloudflare) and produced no event, while
      `https://api.openai.com/v1/models` showed OUR issuer and spooled one event.
- [x] MILESTONE PASSED with Claude Code (2026-09-18), run from this session with
      per-command environment variables only:
      `HTTPS_PROXY=http://127.0.0.1:8899 NODE_USE_SYSTEM_CA=1 NODE_EXTRA_CA_CERTS=<root.crt> claude -p "..."`
      Claude Code worked normally and did NOT reject our certificate. The event
      shows parser=anthropic, model=claude-opus-5, streamed=true, the full prompt,
      the reassembled answer, and token counts. The spool was deleted afterwards
      because it held a real prompt in plaintext (redaction is Phase 3).
      Gemini CLI could not be used: Google rejects the account tier
      ("IneligibleTierError"), unrelated to the proxy.

## Left — Phase 0

- [x] Repo skeleton + CLAUDE.md + .gitignore + git init
- [x] Tooling check (go, php, composer, mitmproxy) — Docker Desktop MISSING
- [x] Go module + `aiul version`
- [x] docker-compose.yml (Postgres, Redis, MinIO)
- [x] docs/DECISIONS.md, docs/SETUP-MAC.md
- [ ] Owner installs Docker Desktop (`brew install --cask docker`) — needs approval, not run
- [ ] Milestone: `aiul version` runs; `docker compose ps` healthy; curl through mitmweb with explicit --proxy in one terminal only

## Left — Phase 3

- [x] Decision applied: only parsed conversations are spooled; housekeeping calls
      on an allow-listed host are decrypted, forwarded and forgotten
- [x] internal/redact rule list v1: anthropic/openai/google/aws/github/stripe/slack
      keys, bearer tokens, JWTs, password-style assignments, PEM private key
      blocks, connection-string passwords, emails, phones, Aadhaar, PAN, cards
- [x] a test per rule, plus ten over-masking cases proving ordinary prompts are
      left alone, plus a test that rule names never leak a value
- [x] wired into `proxy.record`; redaction has no switch to turn it off
- [x] milestone test `TestSecretsAreMaskedButTheProviderGetsTheOriginal`: the
      provider receives the request byte for byte, the client receives the answer
      unmodified, and the stored event has neither the key nor the email while the
      rest of the prompt stays readable
- [ ] Owner verifies the milestone by hand

## Left — Phase 4

- [ ] platform interfaces + darwin implementations + linux/windows stubs
- [ ] forwarder with spool draining, backoff, delete-after-confirm
- [ ] aiul run / install / uninstall / status / doctor
- [ ] health checks, drift re-apply, fail open
- [ ] privilege split recorded in DECISIONS.md
- [ ] milestone: after install and a fresh login, Claude Code runs normally and its
      prompts are captured; uninstall leaves the Mac clean

## Left — later phases

- [ ] Phase 2 — proxy engine (CONNECT, classify, mint, stream, SSE reassembly, parsers, spool)
- [ ] Phase 3 — redaction
- [ ] Phase 4 — endpoint agent (darwin platform impls, install/uninstall/status/doctor, forwarder)
- [ ] Phase 5 — task tagging (lsof → pid → cwd → git branch → task ID)
- [ ] Phase 6 — Laravel ingestion + storage + scoring
- [ ] Phase 7 — Inertia + Vue dashboard

## Blockers / open questions for the user

- **brotli/zstd.** Decided: leave undecoded for now. Such bodies are recorded as
  metadata only, never as rubbish. Revisit if the logs show real captures being
  lost; the decoders would be `andybalholm/brotli` and `klauspost/compress`.
- Gemini CLI is unusable on this account (Google tier error). Use Claude Code,
  OpenCode or Codex for future capture work.

- PHP is 8.4.23 via Herd, not 8.3. Laravel 12 supports 8.4, so we use it (D4).
- Docker Desktop 4.91.0 is already in /Applications and `docker` 29.8.0 works. The
  Homebrew cask refuses to reinstall over it, which is fine: just `open -a Docker`
  when Phase 6 needs the data services.
- Go module path confirmed as `github.com/pkisan/aiul`.
- **Before Phase 2 starts:** rule 11 requires a written comparison of stdlib-only
  vs goproxy vs go-mitmproxy in DECISIONS.md, with a recommendation, confirmed by
  the owner.
- Phase 1 needs one approval from the owner: running `aiul ca trust`, which adds
  our dev root CA to the System keychain. The exact command is shown before it runs.

## Things the user must run by hand

- Phase 1 milestone verification (Safari test) — commands supplied at the end of the phase.

## Machine state — settings currently changed on this Mac

No system proxy. No env vars. No launchd jobs. No /etc/zshenv block.

**Keychain: the owner ran `aiul ca trust` and then `aiul ca untrust` during the
Phase 1 milestone. Confirm with `aiul ca info` — expect "not trusted". If it says
TRUSTED, the untrust step did not complete; run `sudo ./scripts/killswitch.sh`.**

One thing now exists on disk, but changes no setting and is trusted by nothing:

| What | Where | Undo |
| --- | --- | --- |
| Dev root CA (cert + key, key 0600) | `~/Library/Application Support/AIUL/dev-ca/` | `rm -rf ~/Library/Application\ Support/AIUL/dev-ca` |

If the owner runs `aiul ca trust`, add a keychain row here immediately, undone by
`aiul ca untrust` or `sudo ./scripts/killswitch.sh`.

`sudo ./scripts/killswitch.sh` reverts every system change in one command;
`--dry-run` shows what it would do without changing anything.
