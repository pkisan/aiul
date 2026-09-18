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
