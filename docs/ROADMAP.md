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
- **claude.ai's conversation endpoint** is
  `POST /api/organizations/<org>/chat_conversations/<id>/completion`, alongside a
  lot of `event_logging/v2/batch` noise. Parser still to write.

## Where it stands

| Surface | Decrypted? | Conversation recorded? | Why |
| --- | --- | --- | --- |
| **CLI** — Claude Code, Codex, OpenCode | yes | **yes**, proven live | parsers exist for the provider APIs |
| **Direct API** — curl, scripts, SDKs | yes | **yes** | same parsers |
| **Nine more providers** — Groq, DeepSeek, Mistral, xAI, Together, Perplexity, OpenRouter, Copilot API, Cursor's OpenAI-compatible calls | yes | **probably** — parsed from documented shapes, never driven live | one parser covers the OpenAI format |
| **chatgpt.com in a browser** | yes | **YES** — parser built 2026-09-21 | its private endpoint and patch-stream protocol are now parsed |
| **claude.ai, gemini.google.com in a browser** | yes | not yet | endpoints identified; parsers not written |
| **Copilot in VS Code** | yes | expected — untested | the real host `api.individual.githubcopilot.com` is now allow-listed; nobody has driven it yet |
| **JetBrains AI** | no | no | its hosts are not listed; nobody has captured them |
| **Cursor** | **no, and never will be** | **no** | it pins its certificate: `api2.cursor.sh` rejects ours and is tunneled. Metadata only, permanently |
| **Windsurf, Tabnine, Amazon Q, Gemini Code Assist** | **NO** | no | same: not listed |

Two things to take from that table. Everything on the allow-list is at least
*seen*. Anything not on it is invisible by design — rule 3 — and adding a host is
a deliberate act, not an accident.

## The two obstacles, and they are different

**Obstacle A: private endpoints.** The web apps talk to endpoints like
`claude.ai/api/organizations/.../completion` whose request and response shapes are
undocumented and change without notice. Writing a parser needs real captured
traffic, and keeping it working needs a test that fails loudly when the shape
moves.

**Obstacle B: certificate pinning.** Some clients refuse any certificate that is
not the one they expect, whoever signs it. Your own log has an example already:

```
level=WARN msg="client rejected our certificate; tunneling this host from now on"
host=chatgpt.com err=EOF
```

That is rule 4 working — the tool keeps working, and we record metadata only — but
it means **some surfaces cannot be captured at all**, no matter what parser is
written. Browsers are the hard case: Chrome and Firefox have their own trust
stores, and Chrome enforces pinning for some origins.

This has to be said out loud to whoever buys the product: *usage through a pinning
client is countable, not readable.*

## The plan, in order

### Step 1 — Find out what each tool actually talks to (needs you)

The research workflow in CLAUDE.md, once per tool. For each of Cursor, Copilot in
VS Code, JetBrains AI, claude.ai in a browser, chatgpt.com in a browser:

```sh
mitmweb                       # terminal 1, and trust its CA for this test only
HTTPS_PROXY=http://127.0.0.1:8080 <run the tool>
```

Have a short conversation in the tool, then save what it sent. That gives us:
the exact hostnames, the endpoint paths, the request and response shapes, and
whether the tool accepts an inspected certificate at all.

**This is the gate for everything below.** Without captures, any parser I write is
a guess, and a guess that silently records nothing is worse than an honest gap.

### Step 2 — Extend the allow-list, one host at a time

Each addition is versioned (`AllowListVersion`), anchored to whole hostnames, and
never a broad shared domain. Candidates, pending captures:
`api2.cursor.sh` · `repo42.cursor.sh` · JetBrains AI hosts · Codeium/Windsurf
hosts · `q.amazonaws.com` for Amazon Q.

### Step 3 — A parser per surface, against fixtures

One per shape, tested against anonymised captures in `agent/testdata/`, exactly as
the OpenAI, Anthropic and Gemini parsers are.

### Step 4 — A capture matrix, kept honest

A table in the docs and a command — `aiul doctor --matrix` — that says for each
detected tool: allow-listed or not, accepts our certificate or not, parser or not.
So "is Cursor covered?" has an answer that is checked by the machine rather than
remembered by a person.

### Step 5 — Make IDEs trust our CA

An IDE that rejects our certificate gets tunneled and recorded as metadata. Each
needs its own nudge, and the agent already knows the variable names per tool
(`TrustVars` in `internal/platform/tools_darwin.go`):

- **VS Code / Cursor / Electron apps** — `NODE_EXTRA_CA_CERTS`, which the agent
  already sets machine-wide
- **JetBrains IDEs** — the bundled JDK has its own truststore; the CA has to be
  imported into `cacerts`, or the IDE pointed at the system one
- **Browsers** — Chrome uses the system keychain on macOS (so it works), Firefox
  uses its own store and needs a policy file

---

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
- **A named owner for raw-prompt access**, and a reason recorded on every read —
  the product enforces the reason; the policy has to say who is allowed to have
  one.
- **A retention decision per tenant.** The default is 90 days.
- **An integration test for the deployment path.** Seven of seven bugs today were
  in install, upgrade and uninstall, and none were visible to the unit suite.
