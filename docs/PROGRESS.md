# PROGRESS — AI Usage Logger

Single handoff file. Every new session reads CLAUDE.md then this file before doing anything.

Last updated: 2026-09-19

## Current phase

**Phase 7 — Minimal dashboard (Inertia + Vue).**

Task in progress right now: **verifying the privilege split on this Mac** —
steps 1-4 of five are DONE and the split is proven working; step 5 (uninstall and
confirm the Mac is clean) is pending. The owner chose this as the next step on
2026-09-19, ahead of the retention job, per-tenant keys (D9) and Phase 8.

### VERIFIED on this Mac, 2026-09-19

| What | Evidence |
| --- | --- |
| worker is NOT root | `ps`: `_aiul  /usr/local/bin/aiul run --manage-proxy` |
| helper IS root | `ps`: `root  /usr/local/bin/aiul helper --group 448` |
| socket locked down | `srw-rw---- root _aiul /var/run/aiul-helper.sock` |
| worker uses the helper | log: `using the privileged helper` |
| only AI hosts decrypted | log: `decision=pass` for google, icloud, deepseek; `no parser for this endpoint` for chatgpt.com housekeeping |
| task tagging works through the helper | event: `task_id AIUL-99`, `branch feature/AIUL-99-privilege-split`, `process curl` |
| kill switch | ran once for real, restored HTTPS in one command |

Four bugs were found by doing this, none of which any unit test could have found
(`386ac83`, `9fef921`, `0f3b53e`, `e903faa`) — see the write-up below.

Ready for the owner to run (nothing has been applied yet):

```sh
cd ~/Desktop/Aayatti/agent
sudo AIUL_DEV_ALLOW_UNMANAGED=1 ./aiul install --apply
```

### What the three installed runs found

Attempt 1 (07:39) failed and broke HTTPS. Attempt 2 (09:05) came up correctly but
task tagging was silently untagged. Attempt 3 (09:55, with `AIUL_DEBUG=1`) proved
the classification and, after `e903faa`, the tagging.

Two further fixes came out of attempts 2 and 3:

- `AIUL_DEBUG=1` now does what `--debug` does, and install writes it into the job
  definition. Two failures in a row had an empty log because every line that
  explains a non-recorded request is at debug level and launchd starts the worker
  with a fixed argument list (`0f3b53e`).
- `aiul status` was reporting two untruths: `background job no` while both halves
  ran (`launchctl list` as an ordinary user lists only that user's jobs, never
  system daemons — it asks `ps` now), and `events waiting 0` from the owner's
  spool while the worker writes to `/var/db/aiul/spool`. It now reports the
  worker's spool and says plainly when counting it needs root (`0f3b53e`).

### First attempt, 2026-09-19 07:39 — FAILED, and it broke HTTPS on this Mac

The owner ran `install --apply`. The helper came up as root; the worker never
started; install had already set the system proxy, so nothing on the Mac could
reach the internet until `sudo ./scripts/killswitch.sh` ran. The kill switch
restored it in one command and `aiul status` then reported the Mac clean.

Three causes, all fixed in `9fef921`:

1. launchd opens a job's log files as the account the job runs as, and
   `agent.log` / `agent.err.log` were root-owned. launchd could not start the
   worker at all — and because it never ran, it wrote nothing saying why, which
   is what made this confusing. Install now creates those two files and gives
   them to `_aiul`.
2. `AIUL_DEV_ALLOW_UNMANAGED=1` was set in the owner's shell. A launchd job does
   not inherit that, so the worker hit the MDM gate and exited. The job
   definition now carries the environment the worker needs.
3. Install set the system proxy without checking anything was listening. It now
   waits up to 20s for the port to accept a connection and otherwise rolls back
   the proxy, the environment variables and both jobs, naming
   `/var/log/aiul/agent.err.log`.

What worked: the helper removed the system proxy on unload (rule 7), the socket
was `root:_aiul` mode `srw-rw----`, `/var/db/aiul` was owned by the service
account, and the kill switch reverted everything including the `_aiul` account.

Fixed before that attempt (`386ac83`): install read the CA and wrote its path
into the environment variables from `$HOME`, which under `sudo` may be root's
home. `paths.State` now prefers the account named in `SUDO_USER`, and the launchd
CA copy goes through `paths.CADir`. Three tests in `internal/paths`.

Still leftover on the Mac: `/var/db/aiul` (the worker's CA copy, owned by uid
448, which no longer exists). Harmless, and the next install overwrites it.
Remove with `sudo rm -rf /var/db/aiul`.

## Plan for the privilege split

The shape, from D6:

```
aiul helper   root, tiny, no network. Listens on a unix socket and answers a
              fixed set of verbs. Never parses traffic.
aiul run      unprivileged (_aiul). Proxy, TLS, parsing, redaction, spool,
              forwarder — everything that touches bytes from the network.
```

The worker needs exactly three privileged things, so the helper has exactly three
verbs plus a health check:

| Verb | Why the worker cannot do it itself |
| --- | --- |
| `PING` | health check |
| `PROXY-ON` / `PROXY-OFF` | networksetup needs root |
| `PROCESS <port>` | lsof cannot see another user's processes without root |

Steps:

1. `internal/helper`: the protocol, a client and a server. One file each, text
   lines, a fixed verb list, and a port argument validated as an integer.
2. `cmd/aiul/helper.go`: `aiul helper`, root, socket at /var/run/aiul-helper.sock
   owned root:_aiul mode 0660.
3. `aiul run` prefers the helper when the socket exists, and falls back to doing
   it directly when run by hand in development.
4. `aiul install`: create the `_aiul` service account, a spool directory it owns,
   a CA location it can read, and TWO launchd jobs — the helper as root, the
   worker as `_aiul` via the plist's UserName key, so no privilege-dropping code
   is needed at all.
5. Tests: the protocol rejects unknown verbs and malformed arguments, and the
   worker keeps working when the helper is absent.

Phases 0-7 are all done, and the FULL PIPELINE has now been run end to end on this
Mac (2026-09-18): the Go agent captured a real request, masked the secrets in it,
tagged it to the branch's ticket, spooled it, forwarded it to Laravel, and the
queue worker scored it — with no manual step in between. What remains is Phase 8
(signing, packaging, MDM, EDR) and Phase 9 (pilot), plus the debts below.

### The end-to-end run, for reference

```sh
# terminal 1 — backend
cd backend && php artisan serve --port=8088
# terminal 2 — queue
cd backend && php artisan queue:work
# terminal 3 — agent (changes NOTHING on the machine without --manage-proxy)
cd agent && AIUL_DEV_ALLOW_UNMANAGED=1 AIUL_DEVICE_TOKEN='<token>' \
  ./aiul run --endpoint http://127.0.0.1:8088/api/aiul/events
# terminal 4 — drive traffic from a checkout on a ticket branch
HTTPS_PROXY=http://127.0.0.1:8899 NODE_EXTRA_CA_CERTS="$HOME/Library/Application Support/AIUL/dev-ca/root.crt" claude -p "..."
```

Issue a token with `php artisan aiul:provision-device "$(hostname)" --tenant=dev`.

### Before doing anything in a new session

The backend needs its data services running:

```sh
open -a Docker && sleep 40
cd ~/Desktop/Aayatti && docker compose up -d && docker compose ps
```

## Plan for Phase 7

1. Breeze (Inertia + Vue) for auth and the app shell.
2. Roles on users: member, manager, admin. A manager sees aggregates; only an
   explicit permission reveals raw prompt text.
3. Dashboard: interactions per task, AI time per task and per person with the
   definition of "AI time" shown on the page, average scores with their reasons,
   and an explicit "untagged" bucket.
4. A raw-prompt view behind a policy, where EVERY view writes a consent_records
   row naming who looked, at what, and why.
5. A "my data" page where a person sees exactly what was captured about them.
6. Feature tests for the gate and the audit log above all.

Phase 5 is DONE and verified live. The repo is now on GitHub at
github.com/pkisan/aiul (private), pushed 2026-09-18 after GitHub push protection
flagged the redaction test fixtures — fixed by assembling them at run time, and
the two historical strings were allowed through the GitHub UI.

## Plan for Phase 6

1. `docker compose up -d` — Postgres, Redis, MinIO. Needs Docker Desktop running.
2. Scaffold Laravel 12 in /backend, pointed at those services.
3. Migrations: `tenants`, `devices`, `ai_sessions`, `ai_interactions`,
   `quality_scores`, `consent_records`. `tenant_id` on every table, enforced by a
   global scope so a forgotten `where` cannot leak across tenants.
4. Device-token authentication: tokens are hashed at rest, one per device, and the
   ingestion endpoint accepts nothing else.
5. `POST /api/aiul/events`: validates a batch, stores metadata in Postgres, puts
   prompt and answer bodies in MinIO encrypted, replies with the ids it accepted —
   which is exactly what the agent's forwarder deletes on.
6. Horizon job: heuristic quality scoring over six dimensions (clear goal, context
   given, constraints stated, expected output, examples, focus), storing a rubric
   version and the per-dimension breakdown.
7. Feature tests: authentication, tenant isolation, idempotency, task attribution,
   scoring.

Phase 4 is DONE. The owner reported the milestone "went as expected" and ran the
uninstall, and `aiul status` on 2026-09-18 confirms this Mac has NO aiul settings
applied. Note: the `claude -p "say hello"` check produced no spooled event, which
is what you would expect if it was run after the uninstall — if the owner meant it
to be captured, that needs rechecking with the proxy running.

## Plan for Phase 5

Goal: a captured event carries the task ID with zero clicks from the user.

The chain is: the connection's local source port -> the process that owns it ->
that process's working directory -> the git branch there -> a task ID matched by a
configurable regexp (default `[A-Z]+-\d+`).

1. `internal/platform`: a ProcessFinder interface, darwin implementation using
   `lsof`, stubs for linux/windows.
2. `internal/tasks`: read the git branch of a directory (and its parents), extract
   the task ID, with a small cache. No git binary required — read `.git/HEAD`.
3. Wire it into the proxy: the CONNECT handler records the source port, and the
   event gains task, branch, repo and the process name.
4. Tests: temporary git repositories, the regexp, a detached HEAD, a directory
   that is not a repository, and worktrees.

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

Verify the privilege split on this Mac. The order, so an interruption leaves the
Mac working:

1. `./scripts/killswitch.sh --dry-run` — done 2026-09-19, clean, and it covers
   every item install creates (both plists, the `_aiul` account, the binary, the
   trust, the proxy, the `/etc/zshenv` block, the device token). It only *reports*
   `/var/db/aiul` rather than deleting it, which is deliberate.
2. `sudo AIUL_DEV_ALLOW_UNMANAGED=1 ./aiul install --apply` — the owner runs this;
   it prompts for a password so it cannot be run from the session. Attempt 1
   failed and is written up above; attempt 2 is pending. Install now refuses to
   set the system proxy unless the worker is actually listening, so a repeat of
   that failure leaves the Mac working.
3. Check the split actually happened:
   - `ps -o user,command -p "$(pgrep -f 'aiul run')"` — must say `_aiul`, NOT root
   - `ps -o user,command -p "$(pgrep -f 'aiul helper')"` — must say `root`
   - `ls -l /var/run/aiul-helper.sock` — `root:_aiul`, mode `srw-rw----`
   - `sudo ls -l /var/db/aiul/dev-ca/root.key` — owned `_aiul`, mode `-rw-------`
   - `./aiul status` — proxy listening, both jobs loaded, CA trusted, 4 of 4
     network services pointing at 127.0.0.1:8899
4. DONE, and it HAD broken: the event carried the process and working directory
   but no task, because `_aiul` cannot traverse `/Users/vws18` (`drwxr-x---`) or a
   temporary directory (`drwx------`) and so cannot read `.git/HEAD`. `PROCESS`
   now answers with the repo and branch too (D6, `e903faa`). Verified live:
   `task_id AIUL-99`.
5. `sudo ./aiul uninstall`, then `./aiul status` must report the Mac clean again.

Then update the machine-state table below with whatever is still applied.

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

- [x] platform interfaces (ProxyConfigurator, EnvWriter, MDMChecker, ToolDetector,
      ServiceManager) + darwin implementations + linux/windows stubs; all three
      GOOS values build
- [x] platform tests: every mutating operation can print its commands first, the
      /etc/zshenv block editor never eats other content (including a truncated
      block), NO_PROXY covers local addresses, AllManagedVars covers everything the
      agent writes
- [x] forwarder: batches of 50, exponential backoff capped at 15 minutes, deletes
      only what the backend confirmed, sets a corrupt file aside as .bad rather
      than blocking the queue, and ignores HTTPS_PROXY so our own traffic never
      goes through our own proxy (11 tests)
- [x] device token in the macOS keychain via `security`, with linux/windows stubs
- [x] aiul run / install / uninstall / status / doctor
- [x] health checks: removes the system proxy when the proxy is unhealthy, and
      re-applies it when it drifts back
- [x] privilege split recorded in DECISIONS.md (D6), including the known debt that
      `aiul run` is currently one root process
- [x] D7: found and fixed a real bug before it reached the Mac — SSL_CERT_FILE and
      REQUESTS_CA_BUNDLE REPLACE the trust store, so pointing them at our root
      alone would have stopped curl verifying any ordinary website. `aiul` now
      writes `ca-bundle.pem` (the 128 system roots plus ours) and install refuses
      to proceed if it cannot
- [x] MILESTONE: the owner reported it went as expected; uninstall left the Mac
      clean (verified by `aiul status`)

## Left — Phase 5

- [x] platform ProcessFinder using lsof on darwin + linux/windows stubs
- [x] internal/tasks: reads `.git/HEAD` directly (no git binary, nothing executed),
      walks up to the repository root, handles worktrees and detached HEAD, caches
      for 30s so a branch switch is picked up
- [x] wired into the proxy: each event carries task_id, branch, repo, work_dir and
      the process name; a failed lookup leaves the event untagged and never breaks
      the request
- [x] tests over real temporary git repositories (10 in internal/tasks, 2 in the
      proxy), including the over-matching bug found and fixed: `release-2026` was
      being read as the ticket RELEASE-2026, so the default pattern is now
      `\b[A-Z]{2,6}-\d+\b`
- [x] MILESTONE VERIFIED LIVE on this Mac: a request made from a checkout on
      branch `feature/AIUL-42-task-tagging` produced an event with
      `task_id: AIUL-42`, the correct repo, and `process: curl`

## Left — Phase 6

- [x] Docker services healthy. Two fixes: `minio/minio` on Docker Hub now returns
      "pull access denied", so the image comes from `quay.io/minio/minio`; and
      MinIO's S3 port moved to host 9002 because ClickHouse already owns 9000
- [x] Laravel 13.32 scaffolded in /backend (D8), pointed at Postgres 5433,
      Redis 6380, MinIO 9002
- [x] seven migrations: tenants, devices, ai_sessions, ai_interactions,
      quality_scores, consent_records, tenant_id on users. `BelongsToTenant`
      applies a global scope AND stamps tenant_id on insert, so isolation does not
      depend on anyone remembering a `where`
- [x] device tokens: `aiul_` + 48 random characters, stored only as a sha256 hash,
      issued by `php artisan aiul:provision-device`
- [x] POST /api/aiul/events returns the ids it stored, which is exactly what the
      agent's forwarder deletes on. Idempotent; one malformed event is rejected
      without losing the others in the batch
- [x] bodies encrypted in MinIO under `tenant/YYYY/MM/DD/<event id>-<kind>.enc`;
      the row keeps only the key (D9)
- [x] Horizon installed; `ScoreInteraction` scores on the queue with a rubric
      version and per-dimension reasons (D10)
- [x] 22 feature tests, run against real Postgres (`aiul_test`) rather than SQLite,
      because the schema uses jsonb
- [x] MILESTONE VERIFIED END TO END: an event POSTed with a real device token was
      accepted, attached to task AIUL-42 and a new session, its body encrypted in
      MinIO (confirmed unreadable in the bucket), and scored 100 by the queue
      worker with all six dimensions and their reasons

## Left — Phase 7

- [x] Breeze (Inertia + Vue) installed. Two scaffolding problems fixed: Breeze 2.4
      imports `resources/js/bootstrap.js`, which Laravel 13 no longer ships (added
      it); and `breeze:install` OVERWRITES AppServiceProvider and User — the tenant
      bindings and the roles had to be restored, which the isolation test caught
      immediately. A note in AppServiceProvider warns the next person.
- [x] roles (member/manager/admin) plus a separate `can_view_raw_prompts` grant,
      and `AiInteractionPolicy` (D11)
- [x] dashboard: totals, per task, per person, weakest dimensions with reasons,
      untagged as its own visible row
- [x] raw-prompt view: policy-checked, audit-logged BEFORE the text is returned,
      and a typed reason required to read someone else's prompt
- [x] "my data" page, open to every role, listing what was captured and who read it
- [x] `SetTenantFromUser` prepended to the web group so it runs before route-model
      binding — otherwise a cross-tenant row loads and the policy answers 403,
      which admits the row exists. It now answers 404. A test covers this.
- [x] 14 dashboard feature tests; 59 backend tests in total
- [x] MILESTONE VERIFIED LIVE over HTTP: the manager saw AIUL-42 with its score of
      100 and `canViewRaw: false`, was refused the prompt text with 403; the admin
      with the grant was refused WITHOUT a reason, allowed WITH one, and the audit
      log recorded "Arun Admin looked at interaction #1 | reason: support
      investigation | ip: 127.0.0.1"

## Known debts, recorded rather than hidden

- Prompt bodies use one application-wide encryption key. D9 has the plan: a
  per-tenant key from KMS. `BodyStore` is the only class to change.
- Brotli and zstd response bodies are recorded as metadata only.
- Retention (purge bodies after N days, keep the scores) is designed into the
  schema but there is no job that does it yet.
- Local dev seeds three users with the password "password".

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

**Verified clean on 2026-09-18 after the Phase 4 milestone**: no system proxy, no
env vars, no launchd jobs, no /etc/zshenv block, no keychain trust, no installed
binary. `aiul status` reports "This Mac has no aiul settings applied."

Files that exist but change no setting and are trusted by nothing:

| What | Where | Undo |
| --- | --- | --- |
| Dev root CA + combined bundle | `~/Library/Application Support/AIUL/dev-ca/` | `rm -rf ~/Library/Application\ Support/AIUL` |
| Event spool (currently empty) | `~/Library/Application Support/AIUL/spool/` | same |

Docker containers now running (`aiul-postgres`, `aiul-redis`, `aiul-minio`). Stop
them with `docker compose down`; add `-v` to delete their data too.


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
