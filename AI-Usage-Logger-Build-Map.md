# AI Usage Logger — Build Map

2026-09-18 · owner: Punit

## Overview

You are building a module inside your own PM tool that captures every AI interaction on a managed corporate device, attributes it to a user and a project task, scores prompt quality, and reports time and usage. Because the PM tool is yours, tasks and users already live in your database — the capture layer only has to tag each interaction with a task ID you already own.

### The one-picture architecture

AI tools (web, CLI, IDE) all emit TLS traffic to AI provider domains. A userspace endpoint agent routes that traffic through a TLS-inspecting forward proxy. The proxy terminates TLS using leaf certificates it mints from a root CA the company installed on the device via MDM, reads the plaintext prompt and streamed response, redacts secrets, then re-encrypts and forwards to the real provider. Captured events flow to your backend for storage, scoring, and dashboards. For the few tools that refuse interception (certificate pinning), a lightweight tool-level hook recovers the content instead, and the proxy tunnels those connections through untouched.

Main path: AI tool to endpoint agent (sets proxy and trust) to TLS-inspecting proxy (capture and redact) to real AI provider. Parallel path: proxy to backend ingestion to storage, scoring, and dashboard.

### Three principles that govern every decision

**Proxy-first.** One network choke point captures web, CLI, and IDE at once, because it works below the application layer. Per-tool hooks exist only to fill gaps the proxy cannot reach.

**Trust, do not break.** When a client refuses your certificate even with your root fully trusted (true pinning), the proxy passes that connection through untouched and logs metadata only. A logger that breaks people's AI tools does not survive contact with users.

**Signed, not hidden.** The agent's job — redirecting traffic and reading TLS content — is exactly what antivirus and EDR tools hunt for. So you make it a recognized vendor to those tools: code-signed, notarized, allowlisted, deployed through MDM. The goal is the opposite of evasion. This is the line between legitimate corporate DLP and malware.

### Scope boundary

This map covers managed, company-owned devices only. On personal or BYOD devices, system-wide TLS interception is not lawful (you cannot install a root CA on a device you do not own), so that case would be a separate, consent-based, tool-level-only tier — out of scope here.

## Tech stack

The rule: reuse your existing stack (PHP/Laravel) for everything it can do, and reach for another language only where the job genuinely requires it — the proxy and the agent.

| Component | Language / runtime | Why this choice |
| --- | --- | --- |
| TLS-inspecting proxy (MVP) | Python 3.11+ with mitmproxy | Working TLS interception, leaf-cert minting, and a clean addon API in hours, not weeks. Capture logic is written as mitmproxy addons. |
| TLS-inspecting proxy (production) | Go 1.22+ | Rewrite the core only once volume justifies it — Go's TLS stack and concurrency suit a high-throughput proxy. Not day one. |
| Endpoint agent | Go 1.22+ | One codebase compiles to a single static binary for Windows, macOS, and Linux. No runtime to install on the endpoint. |
| Backend (API, ingestion, scoring, admin, billing) | PHP 8.3 + Laravel 12 | Your existing stack and the PM tool itself. Octane for speed, Horizon for queues. |
| Dashboard UI | Inertia + Vue or React | Stays in the Laravel app, no separate API project to maintain. |
| CLI/IDE hooks (for pinned tools) | Go or TypeScript | Small companion capturers for the handful of tools the proxy cannot intercept. |
| Browser extension (Web phase) | TypeScript, Manifest V3 (WXT framework) | Extensions must be JS/TS; WXT handles MV3 and multi-browser builds. Secondary to the proxy. |

### Data infrastructure

PostgreSQL for metadata and interaction records (JSONB for flexible per-tool fields, row-level tenant isolation). Redis for queues, caching, and rate limiting. S3-compatible object storage (AWS S3 in production, MinIO for local dev) for the large, encrypted prompt and response bodies — kept out of hot tables. ClickHouse only later, once you are into tens of millions of events. Billing via Laravel Cashier with Stripe, or Razorpay for India.

### What you install on the dev box

Python 3.11+, Go 1.22+, PHP 8.3 with Composer, Docker (for local Postgres/Redis/MinIO), and mitmproxy. Three repos or a monorepo with three packages: proxy, agent, backend.

## Capture matrix — every AI tool

Every tool routes to the **same proxy** via `HTTPS_PROXY`. The only per-tool difference is which environment variable makes that tool's runtime trust your root CA instead of erroring. Set these at machine level (not a shell rc file) so subprocesses inherit them. Key insight from testing: most of these tools do not truly pin — they fail because their runtime is not reading the OS trust store. That is a config problem you solve, not a wall.

### CLIs

| Tool | Runtime | Route via | Trust the root with |
| --- | --- | --- | --- |
| Claude Code | Bun | HTTPS\_PROXY | NODE\_USE\_SYSTEM\_CA=1 + NODE\_EXTRA\_CA\_CERTS (+ CLAUDE\_CODE\_CERT\_STORE) |
| Codex | Rust | HTTPS\_PROXY | CODEX\_CA\_CERTIFICATE / SSL\_CERT\_FILE |
| Gemini CLI | Node | HTTPS\_PROXY | NODE\_USE\_SYSTEM\_CA=1 + NODE\_EXTRA\_CA\_CERTS |
| OpenCode | Node | HTTPS\_PROXY | NODE\_EXTRA\_CA\_CERTS (docs confirm proxy + custom CA support) |

### IDEs and editors

| Tool | Surface / runtime | Route via | Trust the root with |
| --- | --- | --- | --- |
| Cursor | IDE, Chromium/Electron + Node | System proxy | System CA + NODE\_EXTRA\_CA\_CERTS for the extension host |
| VS Code + AI extension | IDE, Electron + Node | System proxy | NODE\_EXTRA\_CA\_CERTS / system CA for the extension host |
| GitHub Copilot | Runs inside VS Code's Node host | System proxy | Same as VS Code — proxy sees the API calls once CA resolves |
| Antigravity | IDE, Node-based | System proxy | NODE\_USE\_SYSTEM\_CA=1 + NODE\_EXTRA\_CA\_CERTS (Node family) |
| Claude / Codex in IDE | IDE plug-in of the CLI runtime | System proxy | Same vars as the matching CLI above |

### Web apps

| Tool | Surface | Route via | Trust the root with |
| --- | --- | --- | --- |
| ChatGPT | Browser | System proxy | Browser uses the OS trust store — MDM root just works |
| Claude | Browser | System proxy | OS trust store |
| Gemini | Browser | System proxy | OS trust store |
| Copilot (web) | Browser | System proxy | OS trust store |

Web is the easiest surface: browsers honor the OS trust store directly, so the proxy intercepts cleanly with no per-tool vars. This is why Web, despite being listed last to build, is technically the simplest — it reuses the proxy with zero extra trust plumbing.

### The gap and the fallback

A connection that refuses your cert **even with the root fully trusted** is truly pinned. Rule: tunnel it through untouched, log metadata only (domain, timing, bytes), never break it. For high-value pinned tools (Claude Code being the prime candidate for a hook), ship a tool-level capture hook that reads the prompt/response at the application boundary and tags the task from the git branch. The proxy covers the unpinned majority; hooks recover the pinned few that matter.

## Per-OS build map

The backend and proxy logic are OS-agnostic. Everything the **agent** does at the endpoint has one interface and three implementations. Build Linux first to prove the logic, then port. Budget macOS at roughly double the effort of the other two.

| Concern | Linux | Windows | macOS |
| --- | --- | --- | --- |
| Install root CA | Copy PEM to /usr/local/share/ca-certificates/, run update-ca-certificates (RHEL: /etc/pki/ca-trust/, update-ca-trust) | Machine store LocalMachine\\Root, pushed by Intune/MDM | System keychain as trusted root, via MDM configuration profile |
| Runtime still needs env vars? | Rarely, set anyway | Rarely, set anyway | Yes — keychain root does NOT reach Bun/Node runtimes; env vars are essential here |
| Run agent as | systemd unit | Windows Service | launchd daemon (plist) |
| Set system proxy | env vars + /etc/environment | WinHTTP/WinINET (registry + API), ideally via Group Policy/MDM | networksetup / network prefs, or MDM profile |
| Write machine-level env vars | /etc/environment, profile.d | HKLM Environment (registry) | Awkward — no clean system-wide mechanism reaching both GUI and terminal; use a launch daemon or runtime config locations (most trial-and-error) |
| Detect MDM enrollment | No universal concept — rely on your provisioned config / fleet markers | Registry / MDM APIs | profiles status / presence of MDM profile |
| Package format | .deb and .rpm (GPG-signed for repo integrity) | MSI or MSIX (for Intune) | signed .pkg (deployed via MDM) |
| Signing gate | None (no notarization) | EV code-signing cert; submit to Defender for reputation | Developer ID signing + notarize every build or Gatekeeper blocks it |

### Sequencing

Prove the whole pipeline on **Linux** (cheap, scriptable, no signing gate). Then build whichever your pilot customer's fleet skews toward — usually **Windows** next (most common corporate desktop, Intune packaging well-trodden). Do **macOS** knowing it is hardest: notarization gate, plus the keychain-does-not-reach-the-runtime problem, plus the awkward system-env-var story. The CLI trust-var matrix is portable across all three; where and how the agent writes those vars is per-OS work you cannot skip.

## Security handling

Storing people's prompts is a serious liability. Prompts routinely contain API keys, passwords, and customer data. Treat these as core features, not phase-two polish.

### Root CA key management

This private key is the crown jewel — whoever holds it can impersonate any website to every device that trusts it. Generate and store it in an HSM or cloud KMS, never on disk in plaintext. Use a **per-tenant root CA** (one per customer), never a single shared root — a shared root means one leaked key compromises every customer, an unrecoverable event. Per-tenant roots contain the blast radius. Build cert rotation and revocation in from the start.

### Redaction before storage

Runs inside the proxy addon, before anything is written. Detect and mask secrets (API keys, tokens, passwords) and obvious PII in both prompt and response, using a rule-list + keyword approach on text-like bodies only (VibeGuard and Agent Vault are useful reference models). Store the redacted version; never persist raw secrets. A logger that stores plaintext secrets is a liability from the very first captured prompt.

### Encryption and storage

Large prompt/response bodies live in encrypted S3 with per-tenant encryption keys; metadata in Postgres with row-level tenant isolation. Encryption at rest for the bodies is mandatory.

### Access control and audit

Role-based: managers see aggregated metrics by default. Viewing raw prompt text requires a specific permission and every such view is audit-logged. This one rule does more for adoption than any feature — it is the difference between coaching and surveillance.

### Retention

Configurable: e.g. purge raw content after 90 days, keep the derived scores indefinitely. Let each customer set their window.

### Consent and transparency

Even on managed devices, make consent load-bearing in software, not just the employment contract. Employees should be able to see what has been captured about them. On managed devices the agent activates only when it confirms MDM enrollment, so a personal laptop that mistakenly gets the agent will not start decrypting traffic.

### Compliance (get legal review — this is not legal advice)

India's DPDP Act 2023 if operating there; GDPR for EU customers; Chrome Web Store data-disclosure rules for the extension. SOC 2 will come up the moment you sell to larger companies. Offer data-residency options (an India region, an EU region) since enterprises ask. Have a lawyer review before you handle any customer data.

## Signing and packaging

The EV code-signing certificate is one link, not the whole chain. Missing any link below gets your agent quarantined by the customer's own security stack. This is the make-or-break of deployability.

### Beyond the EV cert — the full checklist

**The EV cert itself.** Issued to your legal company entity (expect identity vetting and a hardware token or cloud-HSM key; issuers include DigiCert, Sectigo). Sign every binary — agent, proxy, installers, and the self-updater — not just the main executable.

**Windows, beyond signing.** EV gets you SmartScreen reputation faster, but you still submit binaries to Microsoft Defender so it builds known-good reputation. For a network-filtering component, check whether you need attestation signing. Package as MSI/MSIX for Intune.

**macOS is a different world — EV does not apply.** Enroll in the Apple Developer Program, sign with a **Developer ID** certificate, and **notarize every build** through Apple's service or Gatekeeper blocks it. Package as a signed .pkg for MDM. Design to stay in userspace — if any component needs low-level network access you may hit System Extension entitlements, which Apple grants sparingly.

**Linux.** No notarization or vendor signing gate. Sign your .deb/.rpm packages with your GPG key for repository integrity.

**EDR / antivirus allowlisting (all platforms).** Because the agent behaves exactly like what EDR hunts, proactively submit your publisher identity and binary hashes to the major AV/EDR vendors' allowlisting programs. Provide customers an IT-facing document with your publisher name, cert thumbprint, and per-platform binary hashes so their security team allowlists you on rollout.

**The mental model.** You are not hiding from security software — you are becoming a recognized vendor to it. That is the entire difference between this product and the kernel-evasion approach, and it is why a CISO will approve it.

### When to buy

Do NOT buy the EV cert, HSM, or Apple membership on day one. They are pre-pilot (phase 8 below). Prove the capture pipeline on Linux first — an unsigned dev agent is fine for that. Buy and wire up signing only once the pipeline works and before the agent touches a real corporate machine.

## End-to-end phase sequence

Complete each phase in order — each produces something testable before the next depends on it. Phases 6–7 (backend) are OS-agnostic; everything endpoint-side is Linux-first, then ported. The critical path is phases 2, 4, and 6 (proxy, agent, ingestion).

- [ ] **Phase 0 — Foundations (1–2 days).** Dev box (Ubuntu/WSL2), install Python/Go/PHP/Docker/mitmproxy, three repos, Postgres+Redis+MinIO via Docker Compose. Milestone: curl a site through mitmproxy.
- [ ] **Phase 1 — Root CA infrastructure (2–3 days).** Use mitmproxy's auto-generated dev root to move fast; design (not build) per-tenant roots + HSM/KMS for production. Milestone: dev root trusted in Linux store, browser through proxy shows no warning.
- [ ] **Phase 2 — Proxy engine (1–2 weeks, the core).** mitmproxy addon: classify by SNI (capture / tunnel / pass), read prompt, reassemble streamed SSE response, emit structured event. Maintain the AI-domain list + the tunnel-do-not-break rule. Milestone: run Gemini CLI or OpenCode through the proxy, see full prompt + reassembled response.
- [ ] **Phase 3 — Redaction pipeline (3–5 days).** Mask secrets/PII in the addon before storage, text-like bodies only. Milestone: a fake API key in a prompt is masked in the stored record.
- [ ] **Phase 4 — Endpoint agent (1–2 weeks).** Go binary: set system proxy, install/verify root, write per-runtime env vars, detect installed CLIs, check MDM enrollment, forward events with device identity. Build Linux path first. Milestone: agent installs on clean Linux VM, Claude Code runs through it captured and unbroken.
- [ ] **Phase 5 — Task tagging (2–3 days).** Attach task ID with zero clicks by reading the git branch of the CLI's working directory; agent correlates process to CWD. Milestone: a captured record carries the right task ID.
- [ ] **Phase 6 — Backend ingestion + storage (1–2 weeks).** Laravel: authenticated ingestion endpoint, schema (ai\_sessions, ai\_interactions, quality\_scores, consent\_records), encrypted S3 for bodies, Horizon queues, heuristic quality scoring. Milestone: event lands, appears against the right task, gets a score.
- [ ] **Phase 7 — Minimal dashboard (3–5 days).** Inertia + Vue/React: interactions per task, AI time, scores, role-gated + audit-logged raw-prompt access. Milestone: manager sees usage per project; raw access is logged.
- [ ] **Phase 8 — Signing, packaging, MDM, EDR allowlist (before any pilot).** Buy EV cert + Apple membership, sign everything, notarize macOS, package per OS, move CA key to HSM with per-tenant roots, prepare IT allowlist doc + EDR submissions. Milestone: signed agent installs via MDM on all three OSes without being flagged.
- [ ] **Phase 9 — Pilot (ongoing).** One friendly customer, managed fleet, their IT as your guide. Start with CLIs, prove capture + tagging in real use, then layer Web (simplest — reuses the proxy) and IDE.

### Build order across surfaces

CLIs first (highest-signal usage, forces the proxy backbone), then IDE (reuses proxy + extension-host trust), then Web (technically simplest — browsers trust the OS store natively, but sequenced last because CLIs teach you more). All three surfaces reuse phases 1–6 unchanged.
