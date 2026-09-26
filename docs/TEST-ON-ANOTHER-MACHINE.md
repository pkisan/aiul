# Testing on another machine — macOS, Windows or Ubuntu

> **This is a demo path, not the product.** It lets a colleague see the thing
> working on their own machine in about fifteen minutes: their own backend, their
> own CA, their own data, removed with one command. The real deployment — a
> signed package pushed by MDM, a per-tenant CA chain, a hosted backend — is
> designed in `DECISIONS.md` and not built.

Two commands on every system. The first changes nothing outside Docker; the
second changes the machine, shows every change first and asks before it does.

| | Backend (all three) | Agent | Undo the agent |
| --- | --- | --- | --- |
| macOS | `docker compose -f compose.demo.yaml up -d --build` | `./scripts/enroll-device.sh --user admin@example.com` | `sudo ./scripts/killswitch.sh` |
| Ubuntu (GNOME) | same | `./scripts/enroll-device.sh --user admin@example.com` | `sudo ./scripts/killswitch.sh` |
| Windows 10/11 | same | `powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -User admin@example.com` (Administrator) | `powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1` (Administrator) |

Status of each, as of 2026-09-26:

- **macOS** — verified on two Macs (this one and the Mac mini).
- **Ubuntu** — verified in a container with systemd; not yet on a real GNOME
  desktop with Chrome.
- **Windows** — built and checked on the Mac only; the first real run on
  DESKTOP-Q12UTEE is pending.

## What the tester needs first

- **Docker Desktop** (macOS, Windows) or Docker Engine (Ubuntu), running.
- **This repository**: `git clone`, or copy the folder across. Nothing in it is
  machine-specific — the CA, the device token and the passwords are made on the
  machine it runs on, and none of them are in git.
- **The agent binary in `dist/`**, built on a Mac with `./scripts/build.sh`:
  `dist/aiul-<hash>.pkg` for macOS (from `./scripts/package.sh`),
  `dist/aiul-linux-amd64` or `-arm64` for Ubuntu, `dist/aiul.exe` for Windows.
  If Go is installed on the tester's machine, the scripts build it themselves.
- **Ubuntu only**: `libnss3-tools` (the script installs it if missing).

## Step 1 — the backend

```
docker compose -f compose.demo.yaml up -d --build
```

The first start builds the image, runs the migrations, creates the storage
bucket and the logins. The admin password is printed once, in the log:

```
docker compose -f compose.demo.yaml logs app | grep -i -A1 password
```

Open `http://127.0.0.1:8088/usage` and sign in as `admin@example.com`. Port 8088
taken? Put `AIUL_PORT=8098` in front of both commands (PowerShell:
`$env:AIUL_PORT='8098'` first).

## Step 2 — the agent

Run the command from the table, in the repository folder. It:

1. gets a device token from the Docker backend and links the device to the
   person given with `--user` / `-User`;
2. prints every change it will make and waits for a yes;
3. writes the agent's config (endpoint and token, readable only by the agent and
   administrators), generates a CA for this machine, trusts it, starts two
   background services, sets the environment variables and points the system
   proxy at `127.0.0.1:8899`.

None of these machines is MDM-enrolled, so the script marks it as a test device
(`AIUL_DEV_ALLOW_UNMANAGED=1`). That override prints a loud warning and must
never be set on a real device.

### After it finishes

- **Sign in once as that person and accept the notice.** Until someone who
  accepted it owns the device, the backend discards its events.
- **Open a new terminal**: open ones keep their old environment.
- **Ubuntu**: quit Chrome completely and reopen it (it reads trusted roots at
  start), and log out and back in (`/etc/environment` is read at login).
- **Windows**: Chrome and Edge pick up the proxy by themselves. If they do not,
  look at Settings > Network & internet > Proxy: "Use a proxy server" should be
  on with `https=127.0.0.1:8899`.

## Step 3 — prove it works

Send one prompt from ChatGPT in Chrome or Edge, claude.ai, Claude Code or Codex,
then reload `/usage`.

Nothing appearing? Read the worker's log:

| | Worker log |
| --- | --- |
| macOS, Ubuntu | `tail -f /var/log/aiul/agent.err.log` |
| Windows | `Get-Content C:\ProgramData\AIUL\logs\agent.err.log -Tail 50 -Wait` |

- `decision=tunnel` on an AI host — that client refused our certificate. Its
  traffic passes through untouched; nothing is broken, nothing is recorded.
- `could not forward events` — the backend is not reachable. Events wait in
  the spool and go when it returns.
- `DISCARDED` — the device has no person who accepted the notice (see above).
- nothing at all from that host — it is not on the allow-list in
  `agent/internal/proxy/hosts.go`.

`aiul status` (Windows: `& 'C:\Program Files\AIUL\aiul.exe' status`) says what
is installed and running.

## Removing it

Run the undo command from the table. It is safe to run twice, and safe when
nothing is installed. Add `--dry-run` (Windows: `-DryRun`) to see what it would
do first. The backend goes with `docker compose -f compose.demo.yaml down -v`
(`-v` deletes its data too).

## Pointing at someone else's backend

```
./scripts/enroll-device.sh --endpoint http://192.168.1.50:8088/api/aiul/events --token aiul_...
powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -Endpoint http://192.168.1.50:8088/api/aiul/events -Token aiul_...
```

The token comes from whoever runs that backend:

```
docker compose -f compose.demo.yaml exec app php artisan aiul:provision-device <name> --tenant=dev --user=<email>
```

The backend must be reachable from the tester's machine. Over any real network,
use HTTPS: the device token travels in every request.

## Known gaps, deliberate for a demo

None of these block a demo. All of them block a pilot.

- **Nothing is signed.** macOS `sudo installer` accepts the package; Windows
  SmartScreen may warn about `aiul.exe`; no MDM will push either. Phase 8.
- **No real MDM check on Windows or Linux**: only the developer override.
- **Firefox is not covered on Windows or Linux** (it keeps its own trust list).
- **Windows records no git branch for terminal tools**: another process's
  working directory is not read there.
- **macOS: checkouts under `~/Desktop`, `~/Documents`, `~/Downloads`** record no
  branch without Full Disk Access — see `SETUP-MAC.md`.
