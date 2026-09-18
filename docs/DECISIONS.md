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
