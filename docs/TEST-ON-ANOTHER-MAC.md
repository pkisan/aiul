# Testing on another Mac

Two commands, in this order. The first changes nothing outside this checkout; the
second changes the machine and asks before it does.

```
./scripts/setup-backend.sh     # Postgres, Redis, MinIO, schema, logins, dashboard
./scripts/enroll-device.sh     # the agent: CA, proxy, launchd jobs
```

Undo the second at any time, in one command:

```
sudo ./scripts/killswitch.sh
```

## What the tester needs first

```
brew install php composer node
brew install --cask docker        # then open Docker Desktop once
```

Plus the code: `git clone` this repository, or copy the folder across. Nothing in
it is machine-specific — `.env`, the CA and the device token are all created on
the machine it runs on, and none of them are in git.

## Step 1 — the backend

```
./scripts/setup-backend.sh
```

Starts the three containers, writes `backend/.env` from `.env.example`, generates
`APP_KEY`, runs migrations, creates the MinIO bucket, seeds three logins and
builds the dashboard. Run it as often as you like: every step checks first.

Then leave two terminals running:

```
cd backend && php artisan serve --host=127.0.0.1 --port=8088
cd backend && php artisan queue:work
```

The queue worker scores prompts. Without it everything is still captured and the
score column stays blank.

Sign in at `http://127.0.0.1:8088/usage` as `admin@example.com` with the password
the seeder printed. Only an admin with the raw grant can read prompt text.

### A different port

```
AIUL_PORT=8090 ./scripts/setup-backend.sh
AIUL_PORT=8090 ./scripts/enroll-device.sh
```

`APP_URL` in `backend/.env` is the one place the port lives. Change it there and
both scripts follow.

## Step 2 — the agent

```
./scripts/enroll-device.sh
```

It builds a package if `dist/` has none, provisions a device against the local
backend for its token, prints every change it is about to make, and waits for a
yes. Then it writes `/etc/aiul/agent.conf`, marks the Mac as an unmanaged test
device if it is not MDM-enrolled, and installs.

Pointing at a backend on someone else's machine:

```
./scripts/enroll-device.sh \
  --endpoint http://192.168.1.50:8088/api/aiul/events \
  --token aiul_...
```

The token comes from whoever runs that backend:

```
php artisan aiul:provision-device <hostname> --tenant=<slug> --user=<email>
```

A remote endpoint has to be reachable from the tester's Mac, and the backend must
listen on more than loopback (`php artisan serve --host=0.0.0.0`). Over a real
network, use HTTPS: the device token travels in that request.

## Step 3 — prove it works

Send one prompt from Claude Code, Codex or claude.ai, then reload `/usage`.
Nothing appearing has three usual causes, in this order:

```
tail -f /var/log/aiul/agent.err.log
```

- `decision=tunnel` on an AI host — that client refused our certificate. Its
  traffic passes through untouched; nothing is broken, nothing is recorded.
- `could not forward events` — the backend is not reachable. Events wait in
  `/var/db/aiul/spool` and are sent when it returns; nothing is lost.
- nothing at all from that host — it is not on the allow-list in
  `agent/internal/proxy/hosts.go`.

## What each machine keeps to itself

| Thing | Where | Shared? |
| --- | --- | --- |
| CA and private key | `/var/db/aiul/dev-ca/` | no — generated per machine, never committed |
| device token | `/etc/aiul/agent.conf`, root-only | no — one per device |
| captured events | that machine's Postgres and MinIO | no — unless pointed at a shared backend |
| prompt and answer text | MinIO, encrypted per tenant | no |

Each tester running their own backend means each tester's data stays on their own
machine. Pointing several agents at one backend gives a shared dashboard — which
is what a pilot would do, and what the fleet notes in `SETUP-MAC.md` cover.

## Things this does not do yet

- **The package is unsigned and un-notarized.** `sudo installer` accepts it;
  double-clicking warns, and most MDMs refuse to push it. That is Phase 8.
- **No Full Disk Access.** Checkouts under `~/Desktop`, `~/Documents` or
  `~/Downloads` record no branch — see the section in `SETUP-MAC.md`.
- **No employee notice or consent flow.** Everyone whose prompts are captured
  should know before this is installed, and today nothing in the product tells
  them.
