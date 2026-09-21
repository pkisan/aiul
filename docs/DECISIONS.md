# DECISIONS

Architecture decisions, newest last. Each records what we chose, why, and what we
rejected, so a future session does not re-litigate it.

---

## D1 — macOS first, not Linux (2026-09-18)

**Decision.** Build the endpoint side on macOS first. Linux and Windows come later.

**Why.** The owner's development machine is a Mac. The build map recommended Linux
first because it is cheaper and has no code-signing gate, but that advantage is
worthless if every test needs a VM the owner does not run day to day.

**How we keep the port cheap.** Every OS-specific action sits behind a small Go
interface in `internal/platform` (TrustInstaller, ProxyConfigurator, EnvWriter,
MDMChecker, ToolDetector, ServiceManager). Only the darwin implementation is
written now; linux and windows get stub files that compile and return
"not implemented on this platform". Nothing outside `internal/platform` may call
`networksetup`, `security`, `launchctl` or any other OS tool directly.

---

## D2 — Go from the start; mitmproxy is a research tool only (2026-09-18)

**Decision.** The proxy and the agent are one Go binary, `aiul`. No Python in this
repo, and no "MVP in mitmproxy, rewrite in Go later" step.

**Why.** The build map's two-step plan pays for the capture logic twice: once as
Python mitmproxy addons, once again as Go. The agent has to be Go anyway (single
static binary, no runtime to install on the endpoint), so writing the proxy in Go
too means one language, one binary to sign and notarize, one process to supervise,
and no IPC between agent and proxy.

**Where mitmproxy still earns its place.** As a research tool, run by hand. When we
add support for a new AI tool, the owner runs only that tool through `mitmweb` in a
separate terminal and captures a few real interactions. We anonymise those into
`agent/testdata/` fixtures and write the Go parser against them. The shipped
product never depends on mitmproxy, and it is never configured machine-wide.

---

## D3 — Production CA chain: designed now, built later (2026-09-18)

Development uses a single self-signed root created by our own Go code
(`aiul ca init`), stored under `~/Library/Application Support/AIUL/dev-ca/` with
the private key mode 0600. That is fine for one developer's own machine and is
never used for a customer.

**Plain-language background.** A certificate authority (CA) is just a key pair whose
certificate other software has been told to trust. Anything that key signs is
believed. That is why the root key is the crown jewel: whoever holds it can
impersonate any website to every device that trusts it.

### The production chain (design only — do not build yet)

```
Per-tenant root CA          key lives in KMS/HSM, never on any laptop
        │                   long-lived, offline, one per customer
        │ signs
Per-device intermediate CA  short-lived (days), name-constrained,
        │                   private key generated on and never leaving the laptop
        │ signs
Short-lived leaf certs      minted on demand by the proxy for allow-listed hosts
```

**Per-tenant root, never a shared one.** One leaked shared root would compromise
every customer at once and could not be recovered from. A per-tenant root contains
the blast radius to one customer.

**Root key in KMS/HSM.** The root key object is created inside the HSM and never
exported. Signing is an API call. The laptop never sees it.

**Per-device intermediate.** Each device gets its own intermediate, whose key is
generated on that device (ideally in the Secure Enclave) and never leaves it. If a
laptop is stolen, only that device's intermediate is revoked and it expires within
days on its own.

**Name constraints — the most important control.** The intermediate carries an
X.509 `nameConstraints` extension permitting only our allow-listed AI hostnames.
A correctly implemented client then rejects any certificate from that intermediate
for any other name. So even a fully compromised laptop cannot be used to
impersonate a bank: the constraint is enforced by the verifier, not by our code.

**Rotation and revocation from day one.** Intermediates are short-lived (rotate
automatically, revocation mostly handled by expiry). The root publishes a CRL, and
"revoke this device" is a supported, tested operation — not something invented
during an incident.

---

## D4 — PHP 8.4 (Herd) instead of 8.3 (2026-09-18)

The machine already runs PHP 8.4.23 via Laravel Herd. Laravel 12 supports 8.4, so
we use what is installed rather than adding a second PHP. Revisit only if a
dependency demands 8.3.

---

## Open — to be decided before Phase 2

**Proxy foundation: standard library only, `goproxy`, or `go-mitmproxy`?**
Rule 11 requires a written comparison with a recommendation, recorded here, and the
owner's confirmation before Phase 2 code is written.

**Privilege split (Phase 4).** Which work needs root (system proxy, `/etc/zshenv`,
LaunchDaemon) and which can run as the user (proxy process, spool, forwarder). The
root-privileged surface must be kept as small as possible and documented here.

---

## D5 — Proxy foundation: Go standard library only (2026-09-18)

Rule 11 requires this comparison before any Phase 2 code.

**Decision.** Build the proxy on `net/http`, `crypto/tls` and `crypto/x509` alone.
No proxy framework.

### What we actually need

1. Handle `CONNECT` and decide capture / tunnel / pass **before** any certificate
   exists, from the hostname in the CONNECT line.
2. Mint a leaf per captured host, signed by our CA, cached in memory.
3. Dial upstream with **full** verification against the system roots (rule 5).
4. Stream responses chunk by chunk with immediate flushing (rule 6).
5. Detect a client that rejects our certificate mid-handshake and move that host to
   a tunnel list (rule 4).
6. Read gzip/br/zstd on our copy only, reassemble SSE, run parsers, write events.

Points 4, 5 and 6 are the whole product, and no library provides them.

### The three options

**goproxy.** Small and widely used, but it is built around "hijack the connection
and hand you a `*http.Request`". Its MITM path assumes you want to sign everything
it sees, its certificate handling is its own, and streaming behaviour has to be
fought rather than configured. We would be overriding most of it to get the
per-host classification and the flush guarantees, while still owning all the
capture logic.

**go-mitmproxy.** Closer in spirit (an addon model like mitmproxy's), but it is a
much bigger dependency, less widely deployed, and it buffers bodies by default to
present them to addons — exactly the behaviour rule 6 forbids. Taking it would mean
either accepting buffered responses or patching around its core.

**Standard library.** `http.Server` with a `CONNECT` handler, `net.Dial`,
`tls.Server` with `GetCertificate`, and `io.Copy` on a `TeeReader` is roughly the
same amount of code as configuring either library around its defaults — but every
line is ours, readable, and does exactly what our rules require. Go's `crypto/tls`
is production-grade and is what both libraries use underneath anyway.

### Why the standard library wins here

- **Classification before the handshake.** We decide from the CONNECT hostname.
  Frameworks want to MITM first and let you inspect afterwards.
- **Streaming is non-negotiable.** With `io.Copy` and an explicit `Flush` we can
  prove chunk-by-chunk delivery in a test. With a framework we would be proving the
  framework's behaviour instead.
- **Handshake-failure detection.** We need the error from `tls.Conn.Handshake()`
  itself to add a host to the tunnel list. That is trivial when we own the
  handshake and awkward when a library owns it.
- **Nothing to sign or audit but our own code.** This binary is code-signed,
  notarized and submitted for EDR allowlisting (Phase 8). Fewer third-party lines
  in a TLS-intercepting binary is a direct security and review benefit.
- **The owner is new to Go.** Standard-library code is the code the documentation,
  the books and every future session already understand.

**Cost accepted.** We write the CONNECT plumbing, the certificate cache and the
tunnel bookkeeping ourselves — a few hundred lines, all of them things we would
have had to understand anyway.

**Revisit if** HTTP/2 or WebSocket interception turns into more work than expected;
`golang.org/x/net/http2` (a Go-team package, not a proxy framework) is the first
thing to reach for then, not goproxy or go-mitmproxy.

---

## D6 — Privilege split: what needs root and what does not (2026-09-18)

Rule: the root-privileged surface stays as small as possible, because this binary
runs as root on employees' machines and is exactly the shape of thing an attacker
would like to subvert.

### Needs root

| Action | Why |
| --- | --- |
| `networksetup -setsecurewebproxy` on each service | system network settings |
| Writing `/etc/zshenv` | a file in /etc |
| Writing `/Library/LaunchDaemons/` and `/Library/LaunchAgents/` | system directories |
| `security add-trusted-cert -d` (admin trust domain) | machine-wide trust store |
| Copying the binary to `/usr/local/bin/aiul` | root-owned location |

All of it happens in `aiul install` and `aiul uninstall`, which run once. The
long-running daemon re-applies the proxy setting when it drifts and removes it when
unhealthy — the only root work it does while running.

### Does not need root

The proxy itself, minting certificates, parsing, redaction, the spool, and the
forwarder. These are the parts handling untrusted input — traffic from the network
— and they are the parts that should not be root.

### The split, as built (2026-09-18)

This debt is now paid. The agent is two processes:

```
aiul helper   root. Listens on /var/run/aiul-helper.sock (root:_aiul, 0660) and
              answers four verbs. Opens no network socket, parses nothing that
              came from the network.
aiul run      runs as _aiul. Proxy, TLS termination, HTTP parsing, decompression,
              SSE reassembly, redaction, spool, forwarder.
```

The worker needs exactly three privileged things, so the helper has exactly three
verbs plus a health check:

| Verb | Why the worker cannot do it | Argument |
| --- | --- | --- |
| `PING` | health check | none |
| `PROXY-ON` | `networksetup` needs root | **none** — the address is compiled in |
| `PROXY-OFF` | same | none |
| `PROCESS <port>` | `lsof` cannot see another user's processes, and `_aiul` cannot read anyone's `.git/HEAD` | one integer, range-checked |

### Why PROCESS also reads the checkout (2026-09-19)

The first installed run on the owner's Mac produced an event with the right
process and working directory, and `task_id`, `branch` and `repo` all null. The
cause is ordinary Unix permissions: `/Users/vws18` is `drwxr-x--- vws18:staff`
and a temporary directory is `drwx------`, and `_aiul` is in neither group. The
worker can be told where a process is working and still not be able to look
inside it.

So the privileged half reads it, and `PROCESS` answers `pid`, `name`,
`working dir`, `repo` and `branch` in one reply — one round trip, and the worker
was going to ask for the directory anyway.

This does not widen the helper's surface in the way a path argument would: the
directory is one the helper itself just obtained from `lsof`, never a string the
worker chose. What the helper does with it is `os.Stat` and `os.ReadFile` on
`.git/HEAD`, walking up at most forty levels, executing nothing. Turning a branch
name into a task ID stays in the worker, where the configurable pattern lives, so
the privileged half holds no policy.

The general lesson, worth remembering for Linux and Windows: **unit tests run as
the developer and cannot see this class of bug.** Anything the worker reads from a
person's filesystem has to be checked against the service account's actual
permissions, on a real installed run.

**No verb takes a path, a command, or a hostname.** `PROXY-ON` deliberately takes
no argument: if it accepted an address, anything that could talk to the socket
could redirect the whole machine's traffic. A privileged helper that accepts a
string somebody else chose is an escalation waiting to be found.

**No privilege-dropping code.** launchd starts the worker as `_aiul` through the
plist's `UserName` key, so there is nothing to get wrong in Go — no `setuid`, no
ordering question about which resources were opened before the drop.

**The service account** is created at install: `_aiul`, hidden, `/usr/bin/false`
as its shell, `/var/empty` as its home. It owns `/var/db/aiul`, which holds its
copy of the CA (key still 0600, owner `_aiul`) and the spool. That copy is why
`internal/paths` exists: running by hand uses your home directory, and the
installed worker uses `/var/db/aiul`, and the CA code and the spool code must not
disagree about which.

**Falling back.** Run by hand with no helper listening, the worker does what it
can itself and says so in its log. Task tagging then sees only your own processes,
which is the correct behaviour for a process that is not root.

What is still root: `install`, `uninstall`, and the helper itself. That is the
whole privileged surface, and it is about two hundred lines that never touch a
byte from the network.

### Why the binary is copied to /usr/local/bin

A root daemon must not run from a path a standard user can write. Running it from
a home directory would let that user replace the binary and gain root.

---

## D7 — Which CA variable adds to the trust store and which replaces it (2026-09-18)

This distinction caused a real bug during Phase 4, caught before anything was
applied to a machine.

| Variable | Behaviour | So it gets |
| --- | --- | --- |
| `NODE_EXTRA_CA_CERTS` | ADDS to Node's built-in roots | our root alone |
| `NODE_USE_SYSTEM_CA=1` | tells Node to read the OS store as well | n/a |
| `SSL_CERT_FILE` | **REPLACES** the roots for OpenSSL and curl | the full bundle |
| `REQUESTS_CA_BUNDLE` | **REPLACES** the roots for Python | the full bundle |
| `CODEX_CA_CERTIFICATE` | Rust/rustls | the full bundle, to be safe |

Pointing a replacing variable at our root alone would mean curl could no longer
verify any ordinary website — every connection we deliberately pass through sealed
would fail, and we would have broken the machine while trying not to break a tool.

So `aiul ca` writes `ca-bundle.pem`: the system roots (`/etc/ssl/cert.pem`, or the
Homebrew bundle) plus our root, and the replacing variables point at that. Install
refuses to proceed if the bundle cannot be written, rather than setting a variable
that would break ordinary HTTPS. Tested by `TestBundleContainsSystemRootsAndOurs`
and by verifying curl still reaches example.com using only that bundle.

---

## D8 — Laravel 13, not 12 (2026-09-18)

`composer create-project laravel/laravel` installs 13.32 today. The build map said
12 because that was current when it was written. Nothing in this module depends on
a 12-only behaviour, so we take what ships. PHP is 8.4 via Herd (D4).

---

## D9 — How prompt bodies are stored, and what changes for production (2026-09-18)

**Now.** Prompt and answer text goes to object storage (MinIO locally, S3 in
production), encrypted with `Crypt::encryptString` — Laravel's application key —
under a key shaped `tenant/YYYY/MM/DD/<event id>-<kind>.enc`. The database row
keeps only the object key and the character counts.

**Why not a database column.** Bodies are large and rarely read, and every
dashboard query would carry them. More importantly, retention means "purge raw
content after 90 days, keep the derived scores": with the body in object storage
that is a delete of an object, and the metrics and scores survive untouched.

**Why the tenant is first in the path.** Deleting or expiring one customer's data
becomes a prefix operation, which is what offboarding and a per-tenant lifecycle
rule both need.

**What changed, 2026-09-19 — per-tenant keys are BUILT.** One application key for
every tenant was a single secret whose compromise exposed every customer. Now each
tenant has a random 32-byte data key; bodies are encrypted with it using
AES-256-GCM, and the data key itself is stored wrapped in `tenants.data_key`, so
the database never holds a usable key. It is created on first write under a row
lock — two workers writing a first body for the same new tenant would otherwise
each generate a key, and the second write would make the first body unreadable
forever.

`BodyStore` remained the only class to change, which is why it exists as a class
rather than two calls in a controller.

**The KMS seam.** Wrapping is two private methods, `wrap` and `unwrap`. Locally
they use the application key. In production they become a KMS Encrypt and Decrypt
against the tenant's key, and nothing else in the class knows the difference. The
unwrapped key is cached for the life of the process only — a body write and the
scoring job that follows it both need it, and in production each unwrap is a
network call.

**Bodies written before this.** They used the application key with no marker.
Stored ciphertext now starts with `AIULv2:`, so a body says for itself which key
it needs: the old ones stay readable until retention deletes them, and no
re-encryption pass has to run over object storage. Reading an old body does not
create a key for that tenant.

Eight tests in `tests/Feature/BodyEncryptionTest.php`, the load-bearing one being
that one tenant's data key raises `DecryptException` against another tenant's
body. Verified against real MinIO and Postgres as well as the fake disk.

---

## D10 — What the quality score is, and what it is not (2026-09-18)

Six dimensions, equally weighted, each 0-100: clear goal, context given,
constraints stated, expected output, examples, focus. Every dimension returns a
plain-English reason, stored alongside the score, and the rubric version is stored
with every row.

**It is heuristic on purpose, not a model call.** Three reasons: it must be cheap
enough to run on every interaction; it must be stable, so this month's scores can
be compared with last month's; and it must not send an employee's prompt to a
third party to be judged.

**Equal weighting is honest.** Any other weighting would be invented precision.

**Automated follow-ups are not scored.** A tool result the agent fed itself is not
a prompt a person wrote, and scoring it would drag every average down unfairly.

**It is for coaching, not ranking.** "Your prompts score low on constraints" is
useful. "You are a 42" is not. That is why every score carries its reasons, and why
the dashboard (Phase 7) shows them rather than a bare number.

---

## D11 — Seeing a number is not the same as reading someone's words (2026-09-18)

The dashboard separates two permissions that are usually conflated:

| Who | Sees aggregates | Reads prompt text |
| --- | --- | --- |
| member | their own only | their own only |
| manager | yes | **no** |
| admin | yes | only with `can_view_raw_prompts` granted separately |

A manager being able to see that a task consumed forty prompts is management. A
manager being able to read those forty prompts is surveillance. Making the second
a separate grant — not implied by the role, revocable without demoting anyone — is
what lets a team accept the tool at all.

Three things reinforce it:

- **Every raw view is written to the audit log before the text is returned.** If
  the log write fails, nobody reads anything.
- **A grant-holding admin may open anyone's prompt with one click** (2026-09-21,
  demo request). The reason field is optional and every view is still audit-logged
  — who, whose, when, from which IP — but the "why" may read "no reason given".
  That weakens the accountability story below: say so in the employee notice
  before a pilot, or re-impose the requirement.
- **The person can see who read their words**, on their own "my data" page. The
  audit log is not something only auditors see.

## D12 — The dashboard states how its numbers are defined (2026-09-18)

"AI time" is the summed length of sessions: interactions on one task with no gap
longer than the idle window (30 minutes, configurable). It is not wall-clock time
between the first and last prompt of the day, which would count lunch, and not the
sum of model response times, which would count only the seconds the model spent
typing.

That definition is rendered on the page, not buried in documentation, and a test
asserts it is there. A metric people cannot explain is a metric they will argue
with — and in a tool that measures people's work, that argument is fatal.

Two related choices: automated follow-ups are counted separately from human
prompts, so one question does not look like twenty; and untagged work gets its own
visible row rather than being dropped, because a dashboard that quietly discards
what it cannot classify is a dashboard that lies.

---

## D13 — The agent touches the system proxy only when told to (2026-09-18)

Found while wiring the forwarder to the backend: `aiul run` treated "the system
proxy is not pointing at us" as drift and re-applied it after thirty seconds. So
merely running the agent to try something would have reconfigured the machine's
network settings without anyone being asked — exactly what rule 1 forbids.

The health loop is now behind `--manage-proxy`:

- **without it** (the default) `aiul run` changes nothing: it serves the proxy and
  forwards events, and you point one command at it yourself with environment
  variables
- **with it**, the agent owns the setting: it re-applies the setting if something
  removes it, and removes it if the proxy stops answering (rule 7, fail open)

Only the LaunchDaemon written by `aiul install --apply` passes the flag, because
that is the only path that asked first. On exit the agent removes the proxy setting
only if it was managing it — taking away a setting we never made would be as rude
as making one nobody asked for.

The lesson worth keeping: "re-apply settings that have drifted" sounds like
housekeeping, but on a machine that was never configured, every setting looks like
drift. A reconciliation loop needs to know whether it owns the thing it is
reconciling.

---

## D12 — Brotli and zstd stay undecoded, on evidence (2026-09-19)

**Decision.** Do not add `andybalholm/brotli` or `klauspost/compress`. Bodies in
those formats are still recorded as metadata only.

**Why now.** The condition D-for-revisiting set was "if the logs show real
captures being lost". A full day of real traffic through the installed agent on
the owner's Mac — the Codex and ChatGPT web apps included, which are exactly where
brotli would be expected — produced **zero** `body was compressed in a format we
do not decode yet` lines in 179 KB of debug log. gzip covers what the AI APIs
actually send.

Two dependencies to solve a problem that has not happened once is a cost with no
return. The debug line that would prove otherwise is already in place, so the
evidence will arrive by itself if this is ever wrong.

**Revisit when** that line appears in a real capture, or a new tool's parser needs
a body we cannot read.

---

## D13 — Development users come from a seeder that cannot run in production (2026-09-19)

**Decision.** `database/seeders/DevUsersSeeder.php` is the only thing that creates
the three dashboard users. It refuses to run outside `local` and `testing`, and
the password is random unless `AIUL_SEED_PASSWORD` is given.

**Why.** The three users existed with the password `password`, typed in by hand
during Phase 7. No file created them, so nothing could audit or recreate them.
That is how a demo account reaches production — not because anyone decided it
should, but because nobody could say where it came from.

The three passwords were rotated to a random one when the seeder was written, so
no account on this machine still answers to `password`.

---

## D14 — The device chain is built: how D3 looks in code (2026-09-19)

D3 designed the production chain. This is what was built, with the dev root
standing in for KMS.

```
root CA                  dev root today, KMS per tenant in production
   │ signs               MaxPathLen 1 — exactly one CA below it
device intermediate      key generated ON the device, never leaves it
   │ signs               7 days, MaxPathLen 0, NAME CONSTRAINTS (critical)
leaf certificates        24 hours, minted per host as before
```

**What the name constraints buy.** The intermediate lists the allow-listed domains
in a critical `nameConstraints` extension. A client rejects any certificate from it
for a name outside that list — verified by the client, not by our code. So a stolen
laptop, with the device key in hand, cannot impersonate a bank. It can impersonate
`api.openai.com`, which the fleet's own proxy was already doing on purpose.

The test that carries this is
`TestTheIntermediateCannotBeUsedForAnythingButTheAllowList`: it mints certificates
for `bank.example.com`, `login.microsoftonline.com`, `notopenai.com` and
`api.openai.com.evil.net` and asserts every one is rejected on verification.

**What the short life buys.** Seven days, renewed with two left — enough that a
laptop asleep over a weekend comes back working. A device that leaves the fleet
stops being able to mint anything within a week without anyone revoking it. The
agent renews on start and in its health loop.

**Why the private key never moves.** `Root.SignIntermediate` takes a
`*ecdsa.PublicKey`. There is no field on the request, and no function in the
package, that can carry a private key off the device.

**The KMS seam.** One call: `x509.CreateCertificate` with the root's key becomes an
asymmetric Sign against a KMS key. The template, the constraints and the renewal
are unchanged.

**A root made before today cannot do this.** The dev root carried `MaxPathLen 0`,
meaning "may sign leaves, no further CAs", and an intermediate under it is rejected
by every verifier. `SignIntermediate` detects that and says to run
`aiul ca untrust` then `aiul ca init --force`. The owner's root was regenerated on
2026-09-19 for exactly this reason; the old one was backed up to
`dev-ca.pre-d3-backup` and was trusted nowhere at the time.

**Reissued when the allow-list changes.** A new provider added to the allow-list is
not capturable under an old intermediate — the client would reject our certificate
for it, correctly. `ProvisionDevice` compares the constraints with the current list
and reissues when they differ.

**Still to come, and it needs AWS:** the root itself in KMS, one per tenant, and a
CRL so "revoke this device now" does not mean "wait up to seven days".

---

## D15 — Brotli and zstd are decoded after all (2026-09-21)

**Decision.** Add `github.com/klauspost/compress` and
`github.com/andybalholm/brotli`, and decode both. This reverses D12, on the
evidence D12 itself asked for.

**What changed.** D12 left them undecoded because a full day of real traffic had
produced zero undecodable bodies, and said to revisit "if the logs show real
captures being lost". On 2026-09-21 the log showed exactly that:

```
msg="body was compressed in a format we do not decode yet"
host=claude.ai response_encoding=zstd
```

**Why it matters more than it looks.** An undecoded body does not fail loudly. It
reaches the parser as rubbish, the parser finds nothing in it, and the event is
stored with an empty answer — a record that looks fine and is wrong. Metadata-only
was an acceptable outcome when it applied to nothing; it is not acceptable for a
browser conversation on one of the two surfaces we just built parsers for.

**Cost accepted.** Two dependencies, both widely used, both pure Go, neither
touching the network or the filesystem. Rule 11 prefers the standard library, and
the standard library has neither codec.

**What is still true from D12:** the debug line that produced this evidence stays.
It is what turned "we think this is fine" into "here is the host and the encoding".
