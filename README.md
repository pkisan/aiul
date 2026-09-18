# aiul — AI Usage Logger

Captures AI tool usage on managed company devices, attributes each interaction to
a project task, masks secrets before anything is stored, and reports usage per
person and per project.

The agent and the TLS-inspecting proxy are one Go binary, `aiul`. The backend is
Laravel. **macOS first**; Linux and Windows sit behind the same interfaces and come
later.

> Intended for company-owned, MDM-managed devices. The agent refuses to run on an
> unenrolled device. It is not for personal or BYOD machines.

## How it works

An explicit HTTPS proxy on `127.0.0.1:8899` classifies every CONNECT by hostname,
before any certificate exists:

| Decision | What happens |
| --- | --- |
| **capture** | An allow-listed AI host. TLS is terminated with a certificate minted for exactly the names the real provider's certificate carries; the conversation is read, redacted and spooled. |
| **tunnel** | An AI host that once rejected our certificate. Passed through sealed from then on, metadata only. A tool is never broken twice. |
| **pass** | Everything else — your bank, your email, your updates. Raw bytes, no certificate minted, nothing logged. |

Captured events are tagged with the task automatically: the connection's source
port identifies the process, its working directory gives the checkout, and the
branch name gives the ticket.

## Rules this code is built on

1. **Only AI domains are decrypted.** A versioned allow-list of exact hostnames,
   matched against the whole hostname. No broad shared domains — never all of
   `google.com` or `github.com`. A test fails the build if one is ever added.
2. **Trust, do not break.** A client that rejects our certificate gets tunnelled,
   not broken.
3. **Upstream verification is never weakened.** Our connection to the real provider
   always verifies against the normal system roots. `InsecureSkipVerify` appears
   nowhere, tests included.
4. **Streaming is never delayed.** Responses are forwarded chunk by chunk and
   flushed immediately; the logging copy is taken on the side.
5. **Fail open.** If the proxy is unhealthy, the agent removes the system proxy
   setting rather than blocking traffic.
6. **Redact before storage.** Secrets and obvious personal data are masked on our
   copy before anything is written. The request forwarded to the provider is never
   modified.
7. **One command undoes everything.** `scripts/killswitch.sh`.

## Try it without changing anything

```sh
cd agent
go build -o aiul ./cmd/aiul
./aiul ca init          # a development CA under ~/Library/Application Support/AIUL
./aiul proxy            # listens on 127.0.0.1:8899, changes no system setting
```

Then, in another terminal, point one command at it:

```sh
HTTPS_PROXY=http://127.0.0.1:8899 \
NODE_EXTRA_CA_CERTS="$HOME/Library/Application Support/AIUL/dev-ca/root.crt" \
claude -p "hello"
```

`aiul doctor` explains what is and is not configured. `aiul install` prints
everything it would change and **changes nothing** without `--apply`.

## Layout

```
agent/     the aiul binary (Go, standard library only)
  internal/ca         dev CA, leaf minting, the combined trust bundle
  internal/proxy      CONNECT handling, classification, streaming, SSE
  internal/parsers    one parser per provider
  internal/redact     redaction rules
  internal/tasks      branch to task ID
  internal/platform   everything OS-specific, darwin first
  internal/forward    event spool and backend forwarder
backend/   Laravel (ingestion, storage, scoring, dashboard)
scripts/   killswitch.sh
docs/      DECISIONS.md, SETUP-MAC.md, PROGRESS.md
```

## Status

Phases 0–5 are done: foundations, dev CA, proxy engine, redaction, endpoint agent,
task tagging. Phase 6 (Laravel ingestion and storage) is next. See
[docs/PROGRESS.md](docs/PROGRESS.md) for exactly where things stand and
[docs/DECISIONS.md](docs/DECISIONS.md) for why each choice was made.

Not yet done, and deliberately so: code signing, notarization and packaging
(Phase 8), and the production CA chain — per-tenant root in a KMS/HSM signing a
short-lived, name-constrained intermediate per device — which is designed in
DECISIONS.md but not built.

```sh
cd agent && go test -race ./...
```
