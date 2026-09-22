# ROADMAP — other devices, every AI surface, and the other two operating systems

Three questions, answered honestly, with what it would take.

---

# 1. Putting it on another company-managed Mac

The package works. Three things have to be true on the other machine, and one of
them is not about the machine at all.

## What already works in your favour

- **The MDM gate passes by itself.** That Mac is enrolled, so no
  `/etc/aiul-dev-unmanaged` marker is needed — the agent checks
  `profiles status -type enrollment` and proceeds.
- **The CA is provisioned and trusted per device.** Nothing has to be copied from
  your Mac. That device makes its own root, its own name-constrained
  intermediate, and trusts its own root locally.
- **Uninstall is one command** on the far machine, and so is the kill switch.

## What you have to solve first

**The backend has to be reachable from that Mac.** `127.0.0.1:8088` means "this
machine". Pick one:

| Option | Good for | Cost |
| --- | --- | --- |
| Your Mac's LAN address (`http://192.168.x.x:8088`) | both machines on one network, a quick trial | nothing; breaks when you change network |
| `cloudflared tunnel` or `ngrok http 8088` | a trial across networks, today | a public URL that must not stay up |
| Deploy the backend properly | anything beyond a trial | a server, a domain, TLS, `APP_ENV=production` |

Start `php artisan serve --host=0.0.0.0 --port=8088` for the first two, and open
the Mac's firewall for that port.

**The package is unsigned.** `sudo installer -pkg ...` works and is how you will
do this. A double-click will be refused by Gatekeeper, and **no MDM will push it**
until there is a Developer ID. That is the difference between "I can install this
on a colleague's Mac by hand" and "IT can deploy it to two hundred Macs".

## The steps on the other Mac

```sh
# ON YOUR MAC — build and issue that device a token of its own
cd ~/Desktop/Aayatti
AIUL_VERSION=0.9.0 ./scripts/build.sh && AIUL_VERSION=0.9.0 ./scripts/package.sh
cd backend
php artisan aiul:provision-device "their-macbook" --tenant=dev --user=their@email.com

# Copy dist/aiul-0.9.0.pkg to that Mac, along with the token.
```

```sh
# ON THE OTHER MAC
sudo mkdir -p /etc/aiul
sudo tee /etc/aiul/agent.conf >/dev/null <<'EOF'
AIUL_ENDPOINT=http://<reachable-address>:8088/api/aiul/events
AIUL_DEVICE_TOKEN=aiul_...
AIUL_DEBUG=1
EOF
sudo chmod 640 /etc/aiul/agent.conf

sudo installer -pkg aiul-0.9.0.pkg -target /
aiul status          # expect everything yes, and forwarding=true in the log
```

**One token per device, never shared.** The token identifies the machine; two
machines on one token cannot be told apart and revoking one revokes both.

**Tell the person first.** The agent records what they type into AI tools. There
is an audit log and a "my data" page so that can be done honestly — see
[What has to exist before a pilot](#4-what-has-to-exist-before-a-pilot).

---

# 2. Capturing every AI surface, not just the CLI

This is the real objective, and today the product meets about half of it. Here is
the truth, surface by surface.

## What the 2026-09-21 captures established

Run on the owner's Mac with the agent's own research mode, not mitmproxy.

- **chatgpt.com in a browser is fully capturable.** It accepted our certificate,
  and its conversation endpoint is `POST /backend-api/f/conversation`. A parser
  for it is built and verified against the real capture.
- **Cursor PINS its certificate.** `api2.cursor.sh` rejected ours and was tunneled
  — the tool kept working, and only its telemetry host `api3.cursor.sh` decrypts.
  **No parser can change this.** Cursor usage is countable (when, how much, by
  whom) and never readable.
- **Copilot was invisible for a fixable reason**: a personal plan talks to
  `api.individual.githubcopilot.com`, which was not on the allow-list.
  `api.githubcopilot.com` saw nothing. Now listed, along with the business and
  enterprise prefixes.
- **claude.ai is fully capturable too.** Its endpoint is
  `POST /api/organizations/<org>/chat_conversations/<id>/completion`, and its
  response stream turned out to be the Messages API's, so the existing reassembly
  was reused. Parser built and verified against the real capture.

## Where it stands — updated 2026-09-22

| Surface | Decrypted? | Prompt + answer recorded? | Evidence |
| --- | --- | --- | --- |
| **Claude Code CLI** | yes | **yes** | live, continuously |
| **Claude desktop app** | yes | **yes** | row 420: `prompt_chars=18 answer_chars=110` |
| **Codex over HTTP** | yes | **yes** | row 452: `gpt-5.6-luna`, 43k prompt, answer, tokens |
| **Codex over WebSocket** | yes | **no** | `GET /backend-api/codex/responses → 101`; fixture recorded, no frame reader |
| **chatgpt.com / claude.ai in a browser** | yes | **yes** | parsers built 2026-09-21 |
| **Nine OpenAI-compatible providers** | yes | probably | one parser, never driven live |
| **Gemini CLI / gemini.google.com** | yes | parser exists, undriven | fixtures in `testdata/gemini`, no live capture |
| **Copilot in VS Code** | **no — pins** | metadata only | `api.individual.githubcopilot.com` → `remote error: tls: unknown certificate` |
| **Cursor** | **no — pins, retested 2026-09-22** | metadata only | refused with our CA named explicitly: `remote error: tls: unknown certificate` from `Cursor Helper`, GREASE values in the hello (`tls=0x0a0a`) identify Chromium's stack, which *does* read the keychain our CA is trusted in |
| **Antigravity** | hosts found 2026-09-22 | not yet | talks to `cloudcode-pa.googleapis.com` and its `daily-` twin; both now allow-listed, trust and parser untested |
| **JetBrains AI, Windsurf, Tabnine, Amazon Q** | **no** | no | not allow-listed |

A correction to the earlier version of this table: it said Cursor "never will be"
capturable. That was written before the alert types were distinguishable. Cursor
sends a real `bad certificate` alert, so it does use a private trust store — but
whether an environment variable or a setting can point it at ours is **untested**,
and the difference between "pins" and "has not been asked properly" cost a day
elsewhere in this project. It is an experiment, not a verdict.

## The two obstacles, and they are different

**Obstacle A: private endpoints.** The web apps talk to endpoints like
`claude.ai/api/organizations/.../completion` whose request and response shapes are
undocumented and change without notice. Writing a parser needs real captured
traffic, and keeping it working needs a test that fails loudly when the shape
moves.

**Obstacle B: certificate pinning — and telling it apart from everything that
looks like it.** A client that truly refuses our certificate sends a TLS alert:

```
level=WARN msg="tunneling this host for this program from now on"
host=api2.cursor.sh reason="the client rejected our certificate"
alpn=h2,http/1.1 err="remote error: tls: unknown certificate"
```

A client that simply died mid-handshake sends nothing — a bare EOF or a TCP reset
— and for a day that was read as pinning, which silenced the Claude desktop app
within seconds of every launch. `clientObjected()` now gates rule 4 on an actual
alert, and `helloFingerprint()` records what the client offered. **Any claim that
a tool "pins" must cite the alert**, not an EOF.

Where it is real, rule 4 keeps the tool working and records metadata only.
Browsers are the hard case: Chrome and Firefox have their own trust stores, and
Chrome enforces pinning for some origins.

This has to be said out loud to whoever buys the product: *usage through a pinning
client is countable, not readable.*

## The plan, in order

Ordered by what unlocks the most surfaces per unit of work, not by which tool was
asked about first.

### Step 1 — `aiul doctor --matrix`, so coverage is checked rather than remembered

One command that prints, for every tool it can detect on the machine: allow-listed
or not, sends us an alert or completes the handshake, parser or no parser, and
when it was last seen. "Is Cursor covered?" then has an answer nobody has to
recall, and every step below is measured by how that table changes.

Cheap, and it should come first because it turns the rest into evidence.

### Step 2 — Trust experiments: Cursor, Copilot, VS Code

Each is one experiment, an hour apiece, and the outcome is binary.

For each tool: launch it with the CA variable set explicitly for that process —
`NODE_EXTRA_CA_CERTS` for Electron and VS Code's extension host, `SSL_CERT_FILE`
and `REQUESTS_CA_BUNDLE` for Python-based extensions, the bundled JDK's `cacerts`
for JetBrains — then read the log. Either the handshake completes, and a parser is
all that stands between us and the conversation, or it sends the alert anyway and
that tool is metadata-only until the vendor offers something.

Record each result in the matrix. Do NOT write a parser for a tool that has not
completed a handshake first: that was the mistake pattern of 2026-09-21.

### Step 3 — Two engine capabilities, each unlocking several tools

Both are Phase 2 work on `internal/proxy`, both are bigger than any parser, and
both are prerequisites rather than features:

- **WebSocket** — pass a `101` through while reading frames. Codex needs it
  today; fixture already recorded at `testdata/openai/codex-responses.ws.jsonl`.
- **HTTP/2** — we serve `http/1.1` only. No client has yet been proven to need it
  (`alpn=""` on every failure so far, and Cursor offers both), so this waits for a
  tool that actually demands it. Plan struck once already for lack of evidence;
  do not rebuild it on a hunch.

### Step 4 — A parser per surface that survived Step 2

Cheapest part, and unchanged in method: record a fixture with `mitmweb`, anonymise
it into `agent/testdata/`, write the parser against the fixture, and never against
a shape someone described.

### Step 5 — Antigravity, and anything else new

Nobody here has seen its traffic. It starts where every other surface started: run
it through `mitmweb`, find out which hosts it talks to and in what shape, decide
whether those hosts are narrow enough to allow-list (rule 3 — never a shared
domain), then Steps 2-4 as usual.

### What to tell a buyer, unchanged

Usage through a pinning client is **countable, not readable**. The dashboard shows
that a tool was used, when, for how long and by whom, and cannot show what was
said. Anyone evaluating this should hear that before they see a demo, not after.

# 3. Windows and Linux

Every OS-specific action already sits behind a Go interface in
`internal/platform`, with darwin implemented and the other two returning
`ErrUnsupported`. The module builds for all three today. So this is filling in
files, not redesigning anything.

What has to be written, per interface:

| Interface | Linux | Windows |
| --- | --- | --- |
| `TrustInstaller` | copy to `/usr/local/share/ca-certificates/`, run `update-ca-certificates` | `certutil -addstore Root`, or the CertStore API |
| `ProxyConfigurator` | `/etc/environment` plus `/etc/profile.d/aiul.sh`; GNOME also needs `gsettings` | registry `ProxyServer` per user, and `netsh winhttp set proxy` for services |
| `EnvWriter` | `/etc/environment`, `/etc/profile.d/` | `setx /M`, or the registry `Environment` key |
| `ServiceManager` | a systemd unit, `User=aiul` | a Windows Service, `sc create`, running as a dedicated account |
| `MDMChecker` | there is no MDM: check for the management agent your fleet uses | `dsregcmd /status` for Intune/Entra join |
| `ProcessFinder` | `ss -tnp` or `/proc/net/tcp`, then `/proc/PID/cwd` — **easier than macOS**, it is a symlink | `GetExtendedTcpTable` for the pid; the working directory is **hard** (see below) |
| `ToolDetector` | `$PATH` plus `~/.local/share/applications` | `$PATH` plus the registry uninstall keys |

## The three real difficulties

**1. Windows has no cheap way to read another process's working directory.** On
macOS `lsof` gives it; on Linux `/proc/PID/cwd` is a symlink. On Windows it means
reading the PEB of another process, which is undocumented, needs elevated rights,
and breaks between releases. Task tagging on Windows therefore needs a different
signal — most likely the IDE or shell telling us, or matching the pid to a known
workspace path. **Worth deciding before the port starts, not during.**

**2. Linux has no single proxy setting.** Terminals read `/etc/environment`, GNOME
reads its own `gsettings`, KDE reads another, and Flatpak applications see none of
them. Coverage on Linux will be honest but partial, and the docs should say which
surfaces are covered.

**3. The privilege split changes shape.** systemd has `User=`, `DynamicUser=` and
socket activation, which make the helper/worker split simpler than it is on macOS.
Windows Services have their own model. The D6 design holds; the mechanics differ.

## Where a second OS sits against macOS coverage

Ahead of both: macOS is not finished. Cursor and Copilot are on the owner's Mac,
allow-listed, and recorded as metadata only; Codex's WebSocket transport is
unread. A Windows port would begin from an engine that cannot yet read a frame.

The honest sequencing is therefore Steps 1-4 above **before** either port, unless
a customer's fleet forces the question — in which case Linux, for the reasons
below.

## Suggested order

1. **Linux first.** Closest to macOS, `/proc` makes task tagging easier than
   anywhere, and it is where CI runners and dev containers live — which is a real
   share of AI usage nobody is measuring.
2. **Windows second**, once the working-directory question above has an answer.
3. Each port ends with the same milestone this one did: a real capture from a real
   tool, task-tagged, on the dashboard.

Rough shape of the work, given the interfaces exist: Linux is the smaller half,
Windows the larger, and the deployment plumbing on each will produce its own crop
of bugs that no unit test can see — today produced seven on macOS alone.

---

# 4. What has to exist before a pilot

Not technical, and not optional:

- **An employee notice.** What is captured, what is masked, who can read raw
  prompts, how long it is kept, and how to see your own data. The audit log, the
  policy gate and the "my data" page exist so this can be written honestly.
- **A named owner for raw-prompt access.** Every view is audit-logged (who,
  whose, when, IP); the reason is optional since 2026-09-21, so the log may say
  "no reason given" — decide before a pilot whether that is acceptable, because
  the employee notice has to describe it honestly.
- **A retention decision per tenant.** The default is 90 days.
- **An integration test for the deployment path.** Seven of seven bugs today were
  in install, upgrade and uninstall, and none were visible to the unit suite.
