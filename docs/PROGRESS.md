# PROGRESS — AI Usage Logger

Single handoff file. Every new session reads CLAUDE.md then this file before doing anything.

Last updated: 2026-09-23 (Cursor chat reaches the proxy after a settings change)

## Session resume 2026-09-22 — read this first

**Everything below the resume block is history, newest first.** It is long
because this week found a lot; read the top three sections and stop.

### State of the machine

- Installed agent: `ea254eb`, the same commit as `HEAD`. `main` is **2 commits
  ahead of `origin/main`** (the allow-list change and this file).
- Backend running locally: Postgres 5433, Redis 6380, MinIO 9002 in Docker;
  `php artisan serve` on 8088. 653 interactions, 10 sessions, kinds on 134.
- Research mode is ON — `/var/db/aiul/research` holds decrypted, redacted
  exchanges. Turn it off and delete that directory when parser work pauses.
- Checkouts under `~/Desktop` record no branch until the agent is granted Full
  Disk Access (see SETUP-MAC.md). `~/Herd` and elsewhere are fine.

### What captures prompt AND answer today

Claude Code CLI · Claude desktop app · Codex over HTTP · chatgpt.com · claude.ai

### What does not, and why

| Tool | Reason |
| --- | --- |
| Codex over WebSocket | `101` upgrade; frames unread. Fixture recorded at `agent/testdata/openai/codex-responses.ws.jsonl` |
| Cursor | with the settings in "RESULT: Cursor settings" chat reaches us on HTTP/1.1: `RunSSE` and `/agent/v1/run` (WebSocket) — no parser yet |
| Copilot | sends a TLS alert; never retested with the CA named explicitly |
| Antigravity | trust OK 2026-09-23 after the leaf fix; `cloudcode` parser written, not yet seen live |
| Gemini | parser exists, never driven live |

### NEXT STEP

1. Install `ea254eb`, run `./scripts/tool-experiment.sh antigravity`, and read the
   verdict: does it accept our certificate?
2. If yes — record a fixture through `--research`, write the Cloud Code parser.
   If it sends an alert — record it in the matrix and move to Copilot and VS Code,
   which are the last two untested tools on this Mac.
3. Then `aiul doctor --matrix` (ROADMAP step 1), which makes all of the above
   checkable by the machine instead of remembered.

Everything is a DEMO, not the product — see the framing note below before
proposing signing, MDM or a CA chain as blockers.

## RESULT: Cursor settings bring the chat to the proxy — 2026-09-23 12:08

The experiment below worked. The owner added to Cursor's user `settings.json`
(backup at `settings.json.aiul-bak` in the same folder; restore it and restart
Cursor to undo — the kill switch does NOT cover this file):

```
"cursor.general.disableHttp2": true,
"http.proxy": "http://127.0.0.1:8899",
"http.proxySupport": "override"
```

After a restart the extension hosts (`Cursor Helper (Plugin)`) go through our
proxy, offer no ALPN (HTTP/1.1), and decrypt on `api2.cursor.sh`. One prompt
produced two chat candidates, both "no parser":

- `POST /agent.v1.AgentService/RunSSE` → 200, once (12:07:10)
- `GET /agent/v1/run` → 101 WebSocket upgrade, 40 times (from 12:07:46)

HTTP/2 in the proxy is therefore NOT needed for Cursor. NEXT: read the research
dump for `RunSSE` (and any `/agent/v1/run`) under `/var/db/aiul/research`
(needs sudo), decide which carries the prompt and answer, anonymise a fixture,
write the parser. If the chat is the WebSocket, it joins Codex-over-WebSocket
as one shared piece of work: reading frames after the `101`.

## FINDING: Cursor's chat bypasses the proxy entirely — 2026-09-23 11:45

Research run gave an empty `/tmp/aiul-cursor.flows`: Cursor downloaded a 298 MB
update at 11:41:38 and restarted itself at 11:41:48, dropping the mitmweb env.
The owner's "Hello fellas" was still answered. `lsof` on the live processes shows
why nothing reached us either: the **extension hosts** (`Cursor Helper (Plugin)`)
connect DIRECTLY to port 443 — not through 127.0.0.1:8899 — although their
environment carries `HTTPS_PROXY=http://127.0.0.1:8899`:

```
34.229.67.192, 98.95.185.24  agentn.api5.cursor.sh   (the agent/chat backend)
13.223.143.97                api2direct.cursor.sh
13.248.241.7                 api4.cursor.sh
104.18.19.125                api3.cursor.sh
```

`agentn.api5`, `agent.api5` and `api4` have never appeared in our log. So HTTP/2
in the proxy (plan below) would NOT capture Cursor's chat by itself: the traffic
never arrives. The plan is ON HOLD until an experiment gets the chat to the proxy.

Experiment next (Cursor's own settings, reversible, no system change):
`"cursor.general.disableHttp2": true`, `"http.proxy": "http://127.0.0.1:8899"`,
`"http.proxySupport": "override"` in Cursor's user settings.json, restart Cursor,
send one prompt, grep the agent log for `api5`. If it arrives: allow-list the
hosts that carry chat, and it may even be HTTP/1.1. If it still goes direct, the
only route left is transparent interception (a Network Extension — needs the
Apple entitlement, Phase 8 territory) and Cursor stays metadata-less.

## PLAN: HTTP/2 for Cursor — started 2026-09-23

Evidence: Cursor's chat runs on `api2direct.cursor.sh`, whose client offers ONLY
`h2` (tunnelled sealed since `c724681`). The owner's "Hi there" was answered and
nothing of it reached us.

Steps, each its own commit:

1. **Research first (owner, mitmweb).** `mitmweb` speaks HTTP/2. Run
   `./scripts/tool-experiment.sh cursor --research`, send one prompt, save the
   flows. We need: the chat path, and the body format. Cursor uses Connect RPC
   (`/aiserver.v1.*`), so the body is very likely **protobuf** — binary, with no
   published schema. If so the parser reads the wire format without a .proto
   (strings by field number), which is fragile and must be tested against the
   fixture. That answer decides whether steps 2-4 are worth it.
2. **Serve h2 only to clients that offer nothing else.** `GetConfigForClient`
   returns `NextProtos: ["h2"]` when http/1.1 is absent. Every client that works
   today keeps HTTP/1.1, so no current capture can regress.
3. **Stdlib h2, no new dependency.** A one-connection listener handed to
   `http.Server.Serve` (which sets up h2 itself for a `*tls.Conn` that negotiated
   it), and a handler that forwards upstream through an `http.Transport` with
   system roots (rule 5), flushing every chunk (rule 6) and copying bodies on the
   side to the same `record()` path as HTTP/1.1.
4. **Connect/protobuf parser** for the chat endpoint, from the step 1 fixture.

Step 1 goes through mitmweb, not our agent, so the agent's research mode stays OFF.

## Antigravity live row, and two parser fixes — 2026-09-23

Row 1004 from `9cf51d6`: `tool=antigravity model=gemini-3.1-pro-low kind=human`,
17755/21 tokens. But the stored prompt was Antigravity's `<EPHEMERAL_MESSAGE>`
reminder (appended as a later user message, "not actually sent by the user")
and the answer began with the model's thought summary. Fixed: reminders are
skipped when choosing the prompt and deciding the kind; Gemini `thought: true`
parts are no longer part of the answer (applies to the Gemini parser too).
Test `TestCloudCodeIgnoresRemindersAndThoughts`. Research dumps were deleted by
the owner; research mode still has to be removed from `/etc/aiul/agent.conf`.

## Antigravity parser written — 2026-09-23

Research captures showed Cloud Code is Gemini's generateContent inside an
envelope (`{"model", "request": {...}}` and `{"response": {...}}` per SSE chunk).
`parsers.CloudCode` unwraps it and reuses `Gemini{}.Parse`; the prompt is the
text inside `<USER_REQUEST>`, the model comes from the envelope, and the IDE's
title generator (no tools offered) is `utility`. Fixtures in
`testdata/cloudcode/` (agent-turn, title-utility), anonymised: system prompt
truncated, tool descriptions removed, metadata block, ids and thought signatures
replaced. Test `TestCloudCodeAntigravityTurn`.

Cursor, same run: the owner's "Hi there" got its answer ("Hi there. What can I
help you with?"), yet no chat endpoint appears among the decrypted captures —
only `rgstr`, `extensions-control` and the updater. The chat went over the
h2-only `api2direct.cursor.sh` path. Capturing it needs HTTP/2 in the proxy.

NEXT: install the package with the parser, send one Antigravity prompt, confirm
a row with prompt and answer. Then decide on HTTP/2 for Cursor. Turn research
mode off and delete `/var/db/aiul/research` and `~/aiul-research` when done.

## Both tools now decrypt; h2-only clients tunnelled — 2026-09-23

Installed `e8e425e` and ran the trust experiment for both. **Zero TLS alerts.**

- **Antigravity:** decrypted on `cloudcode-pa` and `daily-cloudcode-pa`. The chat
  endpoint is `POST /v1internal:streamGenerateContent` (200, seen twice), plus
  noise: `loadCodeAssist`, `fetchUserInfo`, `listExperiments`,
  `fetchAvailableModels`, `recordCodeAssistMetrics`, `writeTrajectoryAcls`.
  "no parser for this endpoint". NEXT: fixture from `/var/db/aiul/research`
  (research mode is on; needs sudo to read), then a Cloud Code parser.
- **Cursor:** `api2.cursor.sh` decrypts (155 handshakes), but only
  `aiserver.v1.*` metadata calls (DashboardService, AnalyticsService,
  ReportClientNumericMetrics) — Connect RPC over HTTP/1.1. No chat seen there.
  A `node` process calls `api2direct.cursor.sh` offering ONLY `h2`, and we
  refused it 21/21 with no tunnel, breaking that client (rule 4 violation).
  Fixed: `shouldTunnel()` tunnels a client that offers ALPN without http/1.1,
  test `TestH2OnlyClientIsTunnelled`. Cursor chat likely rides that h2 path, so
  capturing it needs HTTP/2 in the proxy — the first real evidence for it.

## Cursor and Antigravity were never pinning: our leaf was invalid — FIX BUILT 2026-09-23, awaiting install

The owner asked to retry both. Before launching anything, stock Go on this Mac
was pointed through the proxy at `cloudcode-pa.googleapis.com`:

```
x509: "upload.video.google.com" certificate is not standards compliant
```

Stock Go trusts the keychain, so the fault was ours. The minted leaf copied
every SAN from the real provider certificate — for Google that includes
`*.googleapis.com`, `*.docs.google.com`, `*.youtube-3rd-party.com`; for Cursor
`prod.authentication.cursor.sh`. The device intermediate is name-constrained to
the 28 allow-listed hosts, and under RFC 5280 one SAN outside the constraints
invalidates the whole leaf. Every strict verifier refused it: Cursor's Chromium
(`unknown certificate`), Antigravity's Go `language_server_macos_arm`
(`bad certificate`), stock Go. Chrome on chatgpt.com worked only because that
site's real SANs all fall inside the constraints. The Copilot "pin" is probably
the same fault — retest it.

Fix: `CertCache.Get(host)` mints for the one requested host only; `sanNames()`
deleted. Also tighter under rule 3. Regression test
`TestLeafVerifiesUnderNameConstrainedIntermediate` verifies a leaf through a
constrained intermediate the way a client does.

The 2026-09-22 "Cursor pins — stop retesting" verdict below is WITHDRAWN.

Next: build and install, `sudo launchctl kickstart -k system/com.aiul.agent` is
done by the script, then `./scripts/tool-experiment.sh cursor`, then
`./scripts/tool-experiment.sh antigravity`. Note Cursor also calls
`api2direct.cursor.sh` offering only `h2`, which we refuse
(`client requested unsupported application protocols ([h2])`) — the first real
evidence a tool may need HTTP/2.

## WITHDRAWN: the HTTP/2 plan — the premise was wrong (2026-09-21)

A plan to teach the proxy HTTP/2 was written here and is now withdrawn. Two
pieces of evidence killed it, both gathered before any of it was built:

- The fixture. The desktop app's own binary, recorded through `mitmweb`, spoke
  **HTTP/1.1** end to end: `user-agent: claude-cli/2.1.275 (external, sdk-cli)`,
  `POST /v1/messages?beta=true`, streamed, 200.
- The ALPN logging from `b0a756d`. Every failing connection reported `alpn=""` —
  not "h2", nothing at all.

What the plan got right is that `openssl s_client -alpn h2` really is refused by
this proxy. No AI client we have seen needs it. If one ever does, the steps are
in git history at `d5bc9f8`; do not rebuild them on today's evidence.

## Claude desktop app is fully captured — DONE 2026-09-21

Installed `d5311df` (carrying `f8d0ef6`). Row 420, from a message typed in the
Claude desktop app: `path=/v1/messages streamed=true prompt_chars=18
answer_chars=110 response_tokens=35`, answer text present. Prompt AND answer.

What made the difference was `f8d0ef6`: the request-parse failure no longer
discards the response. The app's requests are large enough to pass the 4 MiB
copy cap regularly, and every one of those was costing us an answer we had
already copied in full.

## Cursor answered no, Antigravity answered where — 2026-09-22

Both experiments run with `scripts/tool-experiment.sh`.

**Cursor pins, and this time it is proven rather than assumed.** Launched from a
shell with `NODE_EXTRA_CA_CERTS` and `SSL_CERT_FILE` pointed at our CA, with the
tunnel list cleared first, it still answered:

```
host=api2.cursor.sh reason="the client rejected our certificate"
process="Cursor Helper" alpn=h2,http/1.1 err="remote error: tls: unknown certificate"
hello="tls=0x0a0a/1.3/1.2 ciphers=16(0x7a7a,0x1301,0x1302) curves=5 sigalgs=8"
```

A real TLS alert, and the GREASE values (`0x0a0a`, `0x7a7a`) identify Chromium's
network stack — which reads the macOS keychain, where our CA is trusted. It
rejected anyway. 0 exchanges recorded out of 162 connections to `api2.cursor.sh`.
Cursor is metadata-only until the vendor offers a trust setting; no parser would
change that, and the matrix should stop being asked.

**Antigravity's hosts, found by reading what passed sealed:**

```
8  daily-cloudcode-pa.googleapis.com     the AI backend, daily channel
4  cloudcode-pa.googleapis.com           the AI backend
5  oauth2.googleapis.com                 sign-in
2  antigravity-unleash.goog              feature flags
1  antigravity-ide-auto-updater-…run.app updates
```

The two `cloudcode-pa` hosts are allow-listed (AllowListVersion 2 → 3); the other
three are not, because they carry no conversation. Test asserts the narrowness:
`googleapis.com`, `storage.googleapis.com`, `oauth2.googleapis.com` and
`cloudcode-pa.googleapis.com.evil.net` must all stay out.

Untested and next: whether Antigravity accepts our certificate, and what shape
its Cloud Code requests take.

## Framing: everything so far is a DEMO — noted 2026-09-22

The owner's words: "This is not the final version. It just for demo. We will
build the final version later on."

So the two-command path (`setup-backend.sh`, `enroll-device.sh`), the per-machine
dev CA, the unsigned package, the dev logins and the local backend are all there
to show the thing working on a colleague's Mac in ten minutes — not to become the
deployment. `docs/TEST-ON-ANOTHER-MAC.md` says so at the top.

What that means for future sessions: stop re-raising signing, MDM, the CA chain
and the employee notice as blockers on every piece of work. They are listed once,
in that file and in DECISIONS.md, and they belong to the real build. Judge demo
work by whether it demonstrates the capture honestly, not by whether it could
ship.

## One session layout, labels where they are known — DONE 2026-09-22

Sessions 19 and 20 rendered differently: 20 was captured after kinds existed and
got the turn view, 19 was legacy and got the flat list. Two pages behind one URL
shape, which the owner rightly called out.

One layout now, the flat one: every exchange in time order, each with its prompt
and reply text, model, tokens and score. Where the agent recorded who caused the
request the row carries a label — "you asked", "agent step", "tool's own call" —
and where it did not, the row simply has none. The turn grouping is gone from the
page; `intoTurns()` still runs and still marks the final answer, so nothing was
lost if a turn view is ever wanted again.

The tiles adapt the same way: "You asked / Tool's own calls" when kinds are
known, "Exchanges / Tokens" when they are not.

## Sessions sort by last activity — DONE 2026-09-22

Session 17 sat at the BOTTOM of the list while being actively worked in: 177
interactions, last one seconds old, started at 09:34. The list was ordered by
`started_at`, so a long-running session sinks below every session opened since.

`->latest('ended_at')` instead, and the row leads with the last activity, with
the start time kept beside it. Test sets up the exact shape — one session that
began four hours ago and is still going, one that began later and finished
earlier — and fails on the old ordering.

No new session is created for continuing work, and that is deliberate: the idle
window (30 minutes) is what decides, so a session is one stretch of work rather
than one calendar day. The list just has to show when it was last touched.

## No labels where the answer is a guess — DONE 2026-09-22

The owner's point, and it was right for the data in front of them: on session 19
there is no way to tell their prompt from the agent's. Every row there predates
kind detection, and labelling them from the old boolean produced confident
nonsense — "you asked" on twelve agent steps, an 11-character `<severity>15` as
the reply, their real prompt filed under "before your first prompt".

So the page now has two modes, chosen by the data rather than by hope:

- **Every row has a kind** (captured by `78ee42c` or later): turns, as built —
  you asked → the reply → N steps in between.
- **Any row is legacy**: no "you"/"agent" tags at all, no turn grouping. A plain
  list of exchanges in time order, each showing the prompt and the reply text,
  model, tokens and score. Defaults to exchanges with a real reply (more than 40
  characters back), with "Show all N" for the rest. The footnote says why the
  labels are missing.

Saying nothing beats saying something wrong, and this is the second time today
that principle has been earned the hard way.

Still not installed: `78ee42c` is built in `dist/`. Until it is, every new
session is legacy too.

## The session page reads like a conversation — DONE 2026-09-22

The owner opened session 19 and found the page unusable: their own prompt hidden
under "before your first prompt", the real answer filed as an agent step, the
recap presented as the reply, twelve rows tagged "you" that were the agent's own
loop, and every prompt two clicks away behind "Show 1 agent step" → a model name.

Two causes, and only one of them was the page:

1. **The installed agent was `0497227`** — before kinds existed. All 486 rows
   have `kind = NULL` and fall back to the old boolean, which was backwards. Any
   session captured before `78ee42c` is installed will keep reading wrongly, and
   the page now says so in an amber banner instead of presenting a guess as fact
   (`legacy_kind` on every row).
2. **The page showed no text.** Fixed: `interactionsForSession($session,
   withPreviews: true)` returns the first 300 characters of each prompt and
   answer, flattened to one line. A turn now reads "you asked → the reply →
   N steps in between", with the text in the page and "details" as a quiet link.

Previews are gated on `canViewRawPrompts()` (admin plus the grant, per
`User::canViewRawPrompts`) and recorded ONCE per session view as
`ConsentRecord::KIND_SESSION_VIEW` — not once per row, because an audit log with
forty entries for one visit is an audit log nobody reads. A manager without the
grant sees the structure and no text at all.

Tests: previews appear for a grant-holder and are audited exactly once; a
manager without the grant gets nulls and no audit record. 88 backend tests pass.

## Whose prompt was it: human, agent, utility — DONE 2026-09-22

Session 19 was forty rows of which two were the owner's. The `automated` flag was
not just unhelpful, it was backwards:

```
#643  "fix the problems of both the commits"   automated=1   <- the owner's own prompt
#660  "<severity>N ONLY"                       automated=0   <- a grader the tool ran
```

Because it was set when ANY message contained a `tool_result`, and a client
re-sends the whole conversation every turn — so a person's prompt was marked
automated the moment their conversation had used one tool, while the tool's own
fresh side-calls looked human.

Requests are now classified by their shape, from two facts the body states:

| Kind | Rule | What it is |
| --- | --- | --- |
| `human` | the newest message is text | what the person typed |
| `agent` | the newest message is a `tool_result` | the agent continuing work already asked for |
| `utility` | an agent client offered NO tools | the tool grading a prompt, naming a chat, suggesting a next action |

The tools test is applied only to agent clients: a browser or a plain SDK call
offers no tools either, and there that is simply what a question looks like.

`Result.Kind` and `Event.Kind` carry it, a nullable `kind` column stores it, and
old rows keep a null kind rather than a guess computed from a rule known to be
wrong.

On top of it, turns: `UsageReport::intoTurns()` numbers each row with the turn it
belongs to and marks the one row carrying the reply the person actually read (the
turn's last row with answer text; a utility call can never be it). The session
page now shows a turn as "you asked → N agent steps, folded away → the answer",
with the fixed fixture's tools array so the parser tests exercise real shapes.

Not stored, computed: a turn id in the database would be wrong the moment the
rule improves.

## The unit is the PROJECT, not a ticket — DONE 2026-09-22

Owner's decision: this is not being used for task attribution in the PM tool.
Repo, project, prompt, answer, score. Tickets are out.

Why they had to go: `task_id` came from a `[A-Z]{2,6}-\d+` match on the branch
name. `feature/revised-wordpress-sso` is an ordinary branch name and carries no
key, so on this machine 100% of work was "untagged" — a feature that reported
nothing. A checkout, unlike a ticket convention, every interaction has.

- `UsageReport::perProject()`, `interactionsForProject()`, `secondsPerProject()`
  and `averageScorePerProject()` group by `repo`; the project name is its last
  path segment, the full path kept because two checkouts can share a name.
- `GET /usage/project?repo=...` (a query parameter: a repo is a path, and
  encoding one into a path segment is a fight with no prize) and a
  `Usage/Project` page listing that project's interactions with their branches.
- `/usage` leads with sessions by project and branch; "Per task" became "Per
  project"; the "Untagged" tile became "No project — not run inside a checkout".
- **Sessions are grouped by repo** in `sessionFor`, not by `task_id`. Session 17
  proved the old bug: 54 interactions from two different repositories in one
  session, labelled with whichever came first, because both had a null task_id.

`task_id` is still captured and stored — it costs nothing and a team that does
put keys in branch names still gets them — but nothing in the dashboard reads it.

Tests rewritten: per-project rows with branch counts, work outside a checkout as
its own row, a project page listing only its own interactions, and a new
repository starting a new session. 85 backend tests pass.

## Dashboard: sessions first, pages rebuilt, prompt cap raised — DONE 2026-09-21

Three requests from the owner, all done:

1. **No more truncated prompts.** The 4 MiB copy cap is split in two:
   `maxRequestCopyBytes` 64 MiB (the prompt side, where agents re-send whole
   conversations and 5 MB requests are routine) and `maxResponseCopyBytes`
   8 MiB (the answer side, bounded by what a model generates and held in memory
   for the life of a stream). The truncation note stays for anything past 64 MiB.
2. **Grouped by session.** `UsageReport::sessions()` and
   `interactionsForSession()`, a `session()` action, `GET /usage/session/{id}`
   and a `Usage/Session` page that reads OLDEST first, because a session is a
   conversation. `/usage` now leads with sessions: task, tool, branch, person,
   prompts, AI time, average score. `AiSession::user()` added.
3. **The pages were rebuilt** around four shared pieces in
   `resources/js/Components/Usage/`: `Panel`, `StatCard`, `Score`, `Tag` and a
   `format.js` so a duration or timestamp never reads two ways on two pages.
   Index, Session, Raw, Interaction, Task and Audit all use them.

The raw page also stops dumping 150k characters at a reader: it shows the last
4,000 by default — the person's own words are at the END of an agent's prompt —
with a button for the whole thing and a copy button for each body.

Three new tests: sessions group without counting follow-ups as prompts, a
session page lists its interactions, a member cannot open one. 84 backend tests
pass. Frontend rebuilt (`npm run build`).

## Codex HTTP transport: framing sniffed, answers parse — 2026-09-21

`body_shape` answered it in one line:

```
body_shape="first=6576656e743a2072 data_lines=17 json_start=false"
            "event: r"
```

Event-stream framing, 17 data lines, and no `Content-Type` at all — so
`isSSE()` said no and 207 KB of answer went to the JSON path and parsed as
nothing.

`looksLikeSSE()` now decides by the bytes when the header does not say: a body
beginning `event:` or `data:` is a stream. The OpenAI parser already understood
`response.output_text.delta`; it now also reads usage from
`response.completed`, which is where the Responses API puts it.

Tests: the framing test accepts the three shapes Codex sends and refuses JSON
that merely contains the words; the parser test reassembles a Responses stream
with the event names recorded in `codex-responses.ws.jsonl`.

The WebSocket transport is untouched and still uncaptured — see below.

## OPEN: Codex, and two transports rather than one

Codex is not captured, and the fixture showed why the earlier plan was wrong:
there are TWO transports.

1. **WebSocket.** `GET /backend-api/codex/responses` → `101 Switching
   Protocols`, `upgrade: websocket`. Every prompt and answer is in frames; the
   HTTP exchange has no body at all, which is why fifteen of them logged
   `response_bytes=0`. Fixture recorded:
   `agent/testdata/openai/codex-responses.ws.jsonl`, 36 frames —
   `response.create` from the client, `response.output_text.delta` x17 and
   `response.completed` from the server. The CLI uses this. So does the app.
2. **HTTP, with no Content-Type.** Row 422: `status=200 streamed=true
   prompt_chars=43089 answer_chars=0 response_bytes=207441
   response_copy_bytes=207441 content_type="" sse_events=0`. We hold the whole
   answer and cannot tell what framing it is in, because `isSSE()` keys off a
   header the provider does not send.

`bodyShape()` added to the `recorded` line for exactly this: the first eight
bytes as hex, how many `data:` lines the body has, and whether it starts as
JSON. Framing only, never content, with a test that asserts no payload leaks.

Next: install, send one Codex message, read `body_shape`. If it shows
`data_lines>0`, the fix is to sniff the framing when the header is missing and
the existing SSE path takes over. Then a parser for the Responses events, and
separately the WebSocket transport, which needs `forward()` to pass a 101
through while reading frames.

## OPEN: three distinct reasons an answer is missing (2026-09-21)

With `f3d5c18` installed, one message in each app produced this:

```
recorded host=chatgpt.com path=/backend-api/codex/responses status=101 ... response_bytes=0        x15
recorded host=chatgpt.com path=/backend-api/codex/responses status=200 model=gpt-5.6-luna streamed=true
         prompt_chars=42996 answer_chars=0 response_bytes=204922 response_copy_bytes=204922
recorded host=api.anthropic.com path=/v1/messages status=200 prompt_chars=0 answer_chars=0
         response_bytes=1519 response_copy_bytes=1519 content_type=text/event-stream
parse failed parser=anthropic err="unexpected end of JSON input"
```

Three separate faults, not one:

1. **Codex speaks WebSocket.** `status=101` is a protocol upgrade; after it the
   connection is frames, which `forward()` does not parse at all. Fifteen of them
   in one message.
2. **The Codex Responses API has no parser.** The one `200` carried 42,996
   characters of prompt (parsed) and 204,922 bytes of answer the openai parser
   made nothing of. Needs a fixture and a parser, like chatgpt.com had.
3. **Some request bodies never reach our copy.** `prompt_chars=0` with the
   response copied in full, and `unexpected end of JSON input` from the parser —
   the response was there, the request body was not. Intermittent: the same
   endpoint parses on other connections.

A fourth case is NOT a fault: row 273 had `answer_chars=0` with
`response_tokens=223`, which is what a turn that only calls a tool looks like —
the deltas are `input_json_delta` and the parser keeps only `text_delta`.
Whether such a turn should record something is a product decision.

Diagnostic added for the two that are still unexplained (no behaviour change):
- `sse_events` and `sse_types` on the `recorded` line — distinct event and delta
  types with counts, in first-seen order, types only, never the text. This
  separates "no prose in the stream" from "a stream shape we do not know".
- `request_bytes`, `request_copy_bytes`, `request_encoding` and
  `transfer_encoding`, for the empty-request-body case.

Test: `TestSSETypesSummarisesAStreamWithoutRevealingIt` asserts the summary and
that no `partial_json` content leaks into it.

## OPEN: the desktop app's completion is captured but recorded empty

Corrected claim: after `611ed48` the Claude desktop app is no longer tunnelled
and its PROMPT does reach the database — but only through
`POST /v1/messages/count_tokens`, which carries the prompt text and answers with
24 bytes of `{"input_tokens":N}`. The completion itself,
`POST /v1/messages` streamed, produced NO row (ids 188-193 were all
count_tokens; nothing after matched). A fully captured exchange looks like the
terminal CLI's: `path=/v1/messages streamed=true answer_chars=2197`.

What the log shows for the app in that window: handshakes succeeding, no tunnel
decisions, no warnings, and several `forwarding ended host=api.anthropic.com
err=EOF`. `forward()` calls `record()` unconditionally AFTER the response is
streamed, so an exchange that never records is one that returned early — at
`write request upstream` or at `read response`. An `EOF` from `http.ReadResponse`
means the provider closed without answering, and the most likely reason is ours:
`capture()` holds ONE upstream connection per client connection, so a client that
reuses its connection after the provider has dropped ours gets nothing.

Diagnostic added (no behaviour change):
- `forwarding ended` now carries `method`, `path` and `request_on_connection`,
  so a failure on the second or third request of a reused connection is
  distinguishable from a failure on the first.
- `client connection ended` carries `requests_served`.
- A new `recorded` DEBUG line carries parser, model, streamed, prompt_chars,
  answer_chars, response_bytes, response_copy_bytes and content-type — an event
  with a prompt but no answer is visible nowhere else.

Database was wiped clean on request the same day (0 interactions, 0 sessions, 0
quality_scores, 352 body objects deleted; devices, users, tenants and consent
records kept so the agent keeps ingesting). Everything from here is fresh.

## Silence is not a refusal — DONE 2026-09-21 (the actual cause)

The fingerprint settled it. The SAME executable, with the SAME ClientHello,
succeeded and failed six seconds apart:

```
16:29:34 DEBUG client handshake succeeded  hello="tls=1.3/1.2 ciphers=18(...) sigalgs=9 sni=api.anthropic.com alpn=0"
16:29:40 WARN  tunneling ...               hello="tls=1.3/1.2 ciphers=18(...) sigalgs=9 sni=api.anthropic.com alpn=0"
                                           err="read: connection reset by peer" process_at_failure=""
```

And the app's child environment, finally read off the live process rather than
inferred from its parent, is identical to a shell child's: `NODE_EXTRA_CA_CERTS`,
`NODE_USE_SYSTEM_CA=1`, `SSL_CERT_FILE`, `HTTPS_PROXY` all present.

So the client was never the variable. The ERROR was:

| Client | Error | Meaning |
| --- | --- | --- |
| Cursor (really pins) | `remote error: tls: unknown certificate` | a TLS alert — an objection |
| Claude desktop app | `EOF`, `read: connection reset by peer` | a process that exited |

A client that distrusts a certificate says so; TLS has alerts for exactly that.
A bare EOF or a TCP reset is the kernel tidying up after a process that is gone —
and `process_at_failure=""` on every one of those lines says the same. The
desktop app spawns short-lived children; they connect, send a ClientHello, exit,
and rule 4 read the silence as pinning and condemned the executable for every
later connection, including the ones carrying prompts.

`clientObjected()` now gates rule 4: `tls.AlertError` or `remote error: tls:`
tunnels; EOF, reset and timeout are logged at DEBUG and recorded nowhere. The
`helloSeen` guard from the previous commit is subsumed by it (no hello, no
alert) and its test still passes. New test covers both lists of errors, and
removing the gate makes the integration tests fail.

Three wrong theories preceded this one — HTTP/2, stripped environment variables,
pre-warmed connections — each plausible, each fixed something real, none of them
the cause. What ended it was logging the ClientHello and re-reading the error
text, not another hypothesis.

## DIAGNOSTIC: fingerprint the refusing client — 2026-09-21, awaiting evidence

The Claude desktop app still refuses our certificate on a clean agent with an
empty tunnel list, and every cheap explanation is now excluded:

| Theory | Killed by |
| --- | --- |
| HTTP/2 only | recorded fixture is HTTP/1.1; every failure reports `alpn=""` |
| pre-warmed connection, no ClientHello | guard installed, fires zero times |
| Electron strips `NODE_EXTRA_CA_CERTS` | live Electron pid has all four variables |
| bad CA bundle | `ca-bundle.pem` holds 129 certs, ours among them, keychain trusts it |
| the binary itself pins | same binary from a shell captured fine (row 140, "say OK") |

So the difference is between the app's child and a shell's child of the SAME
executable with the SAME trust material, and no theory left is worth another fix.
This commit adds evidence instead of a fix:

- `helloFingerprint()` records the ClientHello in one log field — TLS versions,
  cipher count and first three suites, curve and signature-algorithm counts, SNI,
  ALPN count. Enough to tell Node from Chromium from Go from curl; nothing about
  the conversation inside.
- The same field is logged at DEBUG for handshakes that SUCCEED, so the refusing
  client can be diffed against a working one.
- The owning process is resolved a second time AT the failure. The first lookup
  happens before the handshake, and a source port is reused fast enough that the
  answer can already belong to a dead process — which is how one second of log
  blamed `process=""` and `process=claude` for the same failure.

Nothing about behaviour changes. Next: install, relaunch the app, send one
message, then compare the two `hello=` lines.

## Pre-warmed connections were being read as pinning — DONE 2026-09-21

`alpn=""` meant `GetConfigForClient` never ran: the client opened the CONNECT
tunnel and closed it again **before sending a ClientHello**. It never saw our
certificate, so it cannot have refused one — but rule 4 recorded it as a refusal
and tunneled that program for every later connection.

Clients pre-warm connections and cancel them. The Claude desktop app does it on
launch, so it silenced itself within seconds of starting, every launch, and its
prompts were never captured. The terminal binary made one connection and used
it, which is why the same executable worked by hand and failed under the app —
and why the hunt went through trust stores, CA environment variables, process
names and HTTP/2 first.

`capture.go` now records a `helloSeen` flag in `GetConfigForClient` and only
calls `AddTunnel` when the client actually spoke. A connection that says nothing
is logged at DEBUG and judged on its next attempt. Test:
`TestACancelledConnectionIsNotTakenForARefusal` opens a CONNECT, hangs up before
TLS, asserts nothing is tunneled and that the next connection is still captured.
Verified it fails without the guard. Proxy tests pass with `-race`.

NOT installed yet: needs a build, package and install, then a relaunch of the
desktop app and one message to confirm a row.

## The desktop app was never pinning: it speaks HTTP/2 — DONE 2026-09-21 (log honesty)

A prompt typed in Claude Desktop at 15:44 produced no row. The log said "client
rejected our certificate", so the hunt went through trust stores and CA
environment variables — and the app had all of them
(`ps eww` shows `NODE_EXTRA_CA_CERTS`, `NODE_USE_SYSTEM_CA`, `HTTPS_PROXY`).

The real cause, proved directly:

```
$ openssl s_client -proxy 127.0.0.1:8899 -connect api.openai.com:443 -alpn h2
SSL alert number 120 — tlsv1 alert no application protocol
```

We advertise `http/1.1` only. The desktop app's bundled Claude Code (2.1.275)
opens `/v1/messages` with ALPN `h2`, so the handshake ends before any
certificate is judged. The same process IS captured on
`/api/claude_cli/bootstrap`, `/api/claude_code_grove` and
`/api/claude_code_penguin_mode`, which it opens over HTTP/1.1. The terminal CLI
(2.1.267) uses HTTP/1.1 throughout, which is why it has always logged.

Done now: the ALPN the client offered is recorded from the ClientHello and the
warning says which of the two failures it was — `handshakeFailure()` in
`capture.go`, with a test that an h2-only offer is not reported as distrust and
that http/1.1-or-nothing still is. Behaviour is unchanged: rule 4 still tunnels,
so the desktop app stays metadata-only until the proxy speaks HTTP/2.

## Dashboard clock was 5h30m fast — DONE 2026-09-21

Every screen showed IST plus another 5h30m: a 15:29 interaction read 8:59 PM.
The agent sends RFC 3339 with the device's offset (`...T15:29:42+05:30`),
`EventIngestionController` did `Carbon::parse($event['time'])` with no
conversion, and `occurred_at` is a plain timestamp column in an application
running in UTC — so the device's wall clock went in as if it were already UTC,
and the browser added the offset a second time on the way out. One `->utc()` at
ingestion fixes it for every screen, because everything reads that column.
Test posts `+05:30` and asserts `09:59:42` is stored; it fails without the fix.
80 backend tests pass.

Rows written before this (ids 1..71 on this Mac) are still 5h30m fast. Not
corrected: the shift to apply depends on each device's offset at the time, and
these are development rows. Say the word and they can be moved by hand.

## The executable had to cross the helper socket — DONE 2026-09-21

Installed `68d9a08` and the log said `process=claude executable=""`. The
executable lookup was written in `platform.DarwinProcess`, but the INSTALLED
worker never calls it: it cannot run lsof as a service account, so it asks the
root helper over the unix socket, and the reply carried five fields — pid, name,
dir, repo, branch. The new field simply never crossed. Same lesson as the seven
deployment bugs: what the worker can do by hand is not what it does installed.

`PROCESS` now replies with a sixth tab-separated field, the executable. The
client accepts five or six, so a worker from the new build keeps working against
a helper from the old one during the moments of an upgrade. Round-trip test
covers a path with spaces. Full suite and `-race` pass.

Known gap: when lsof cannot identify the process at all, the identity is empty
and every such connection shares one tunnel bucket per host. A tool that rejects
our certificate while unidentified therefore tunnels other unidentified
connections to that host. Ranked below getting the identified case right.

## Executable-keyed tunnel list + GUI environment — DONE 2026-09-21

Installing the per-program tunnel list proved it only half fixed things, and
showed the other half. Two bugs, both in the same 15:08 failure:

1. **The desktop app and the terminal CLI share a process name.** Claude Desktop
   bundles its own Claude Code at
   `~/Library/Application Support/Claude/claude-code/2.1.275/claude.app/Contents/MacOS/claude`,
   and `lsof` calls it "claude" — exactly what it calls the terminal CLI. The
   desktop copy rejected our certificate and took the CLI down with it again.
   `Process` now carries `Path` (from `ps -p <pid> -o comm=`) and `Identity()`
   returns it, falling back to the name; the tunnel list is keyed by that.

2. **GUI applications never received our CA.** `launchctl getenv
   NODE_EXTRA_CA_CERTS` was empty for the logged-in user. `install` did call
   `launchctl setenv`, but it runs under sudo, and launchctl writes the domain of
   whoever asks — so everything landed in root's domain. `/etc/zshenv` covers
   shells only, so the desktop app saw the system proxy but had no CA to verify
   it with, and could do nothing except refuse. Both `setenv` and the LaunchAgent
   load now go through the logged-in user's GUI domain (`launchctl asuser <uid>`,
   `launchctl bootstrap gui/<uid>`), with the uid read from the owner of
   `/dev/console`. Uninstall reverses both domains. `scripts/killswitch.sh`
   already did the `asuser` form — only the Go path was wrong.

Three tests: `Identity()` separates the two claudes, `guiSetenvArgs` uses the
user's domain and does not become "asuser 0" with nobody logged in, and the
classifier keeps capturing the CLI after the desktop app is tunnelled. Full agent
suite passes, `-race` clean on proxy and platform.

Built but NOT installed: `dist/aiul-<next>.pkg` still to be produced from this
commit. After installing, Claude Desktop needs one quit and relaunch to pick up
the environment.

## Per-program tunnel list — DONE 2026-09-21

Capture of `api.anthropic.com` stopped at 12:58 today and nothing new reached the
dashboard. Cause: rule 4's tunnel list was keyed by hostname alone. Claude Desktop
starts, pins certificates, drops our handshake (`err=EOF`), and from that moment
every program on the Mac — the Claude CLI included — was passed through sealed for
that host. The same had happened to `api2.cursor.sh`, `chatgpt.com`,
`chat.openai.com` and `api.individual.githubcopilot.com`.

Pinning is a property of the program, not the host, so the list is now keyed by
host plus client program name (`lsof` gives "Claude" for the desktop app and
"claude" for the CLI). `Classify` and `AddTunnel` take that name;
`TunnelHosts` reports `host (program)`; `contextOf` was split so the process
lookup happens once per connection and feeds both the tunnel check and the task
tagging. `aiul status` shows each tunnelled pair. Unit test covers the exact
regression: desktop tunnelled, CLI still captured. Proxy tests pass with `-race`.

Known limits: two tools sharing a process name (two `node` CLIs) share a bucket,
and the list is still in memory, so a restart retries every pinned program once.

NOT yet installed on this Mac — needs a rebuild and reinstall (rule 1, owner's
explicit yes).

## Dashboard drill-down — DONE 2026-09-21 (demo request)

The aggregates page had no click path to a prompt. Now: per-task rows link to
`GET /usage/task/{task}` (`untagged` addresses the no-ticket bucket), each
listing that task's interactions with links to `/usage/{interaction}`, which
already held the audit-logged "Open the prompt text" button. A "Recent
interactions" list on `/usage` gives the short path. Manager gate on all of
it; raw-text policy untouched. `UsageReport::interactionsForTask` + `recent`,
`UsageDashboardController::task`, `Task.vue`, 4 new tests — 77 backend tests
pass. Rebuilt frontend (`npm run build`) so `public/build` serves it.
Follow-up `e43471e`: the "Open the prompt text" button sent no reason, so the
server bounced it back with an error nobody displayed — it looked dead. The
interaction page now has a reason field whose value travels in the link, and
shows the server's error when it still bounces.
One-click admin reads — DONE 2026-09-21 (demo request): a grant-holding admin
opens anyone's prompt with no reason typed. Every view is still audit-logged,
but the "why" may be "no reason given" — recorded in D11 as a deliberate
weakening to revisit in the employee notice before a pilot.
"Empty, not purged" — DONE 2026-09-21: the raw page blamed retention for EVERY
missing body, but rows like #41 never had an answer stored (`answer_chars=0`,
`BodyStore::put` skips blank text). The page now says "No answer text was
captured in this exchange" unless chars were recorded and the key is gone.
Nothing was ever purged early — retention is 90 days, all stored objects verify
present. 79 backend tests.

Task-page pagination — DONE 2026-09-21: `interactionsForTask` paginates 20 per
page (`through` keeps the shape), Task.vue renders page links with
Inertia partial reloads (`only=['interactions']`, scroll preserved). 81 tests.
## MILESTONE: the whole pipeline proven through the INSTALLED package, 2026-09-20

A real `claude -p` run, captured by the agent installed from `aiul-0.9.0.pkg`, is
in the database as interaction #4:

```
host      api.anthropic.com     parser   anthropic     model  claude-opus-5
task_id   AIUL-100              branch   feature/AIUL-100-first-capture
repo      /private/tmp/aiul-test  tool   cli           score  86
redacted  [email]               prompt_chars 1239
```

Nobody typed the task ID. It came from the connection's source port, through the
root helper to the process, to its working directory, to `.git/HEAD`. The body in
MinIO begins `AIULv2:eyJpdiI6...` and decrypts to the prompt with the email
replaced by `[REDACTED:email]`, while the provider received the original bytes.
`example.com` through the same proxy still showed `CN=Cloudflare TLS Issuing ECC
CA 3` — its real issuer, sealed, nothing recorded.

### Seven bugs found by doing it, none of which the tests could see

All in deployment plumbing; the capture path was right the first time it was
reached.

| # | Bug | Commit |
| --- | --- | --- |
| 1 | CA path read from `$HOME` under `sudo` | `386ac83` |
| 2 | worker log files root-owned, so launchd could not start the worker — and it therefore logged nothing | `9fef921` |
| 3 | install set the system proxy without checking the worker was up | `9fef921` |
| 4 | `_aiul` cannot read anyone's `.git/HEAD`, so every event was untagged | `e903faa` |
| 5 | `installer` passes no environment to package scripts, so the endpoint never arrived | `223e3cf` |
| 6 | installing over a running agent did not restart it; then `bootout`+`load` raced and left nothing running, while `waitForProxy` was fooled by the dying process's socket | `c8141fb`, `4322322` |
| 7 | env vars pointed `SSL_CERT_FILE` into `/var/db/aiul`, which only `_aiul` can enter — `curl` broke for every user on the Mac | `ca01dd5` |

The lesson, for Phase 9 and for the Linux and Windows ports: **unit tests run as
the developer, in directories the developer owns, in a process that inherited the
developer's environment.** The installed agent has none of those. Everything that
touches launchd, `installer`, file ownership or the environment has to be proven
on a real installed run.

**Next thing to build:** an integration test for the deployment path — install,
verify, upgrade, uninstall against real launchd — because the unit suite
structurally cannot see any of the seven.

## Current phase

**Phase 7 — Minimal dashboard (Inertia + Vue).**

Task in progress right now: none. **The privilege split is VERIFIED on real
hardware** — all five steps done on 2026-09-19, including the uninstall, and this
Mac is clean again. **The retention job is DONE** the same day (see below).

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
| uninstall leaves nothing | no processes, no socket, no plists, no `_aiul` record, no `/etc/zshenv`, `aiul status` says "no aiul settings applied", internet works |

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

### Where the plan lives

`docs/ROADMAP.md` answers the three questions the owner asked on 2026-09-20:
putting the agent on another managed Mac, covering every AI surface rather than
only the CLI, and what Windows and Linux need. It also lists what must exist
before a pilot that is not code.

Updated 2026-09-21 after the first capture session: **chatgpt.com and claude.ai
in a browser are now parsed** — both verified against real captures taken with the
agent's own research mode rather than mitmproxy. **Cursor pins its certificate**
and can never be read, only counted. **Copilot** was invisible because a personal
plan uses `api.individual.githubcopilot.com`; now allow-listed, still undriven.

### How to test the whole thing

`docs/TESTING.md` is the step-by-step runbook: services up, token issued, package
built and installed, a tagged and redacted capture, the dashboard's gate and audit
log, fail-open, retention, and putting the Mac back. It also states plainly what
the test does not cover.

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

## Retention — DONE 2026-09-19

`php artisan aiul:purge-bodies` deletes prompt and answer bodies once they are
older than that tenant's `retention_days`, and keeps everything derived from them.
The schema already allowed it (a null `prompt_object` means purged) and
`retention_days` was already on `tenants`, so this is one command, one scheduled
entry and a test file — no migration, no new model, no new service.

- object first, row second, so an interruption leaves a key pointing at a missing
  object, which `BodyStore::get` already reads as purged — never a row claiming
  nothing is stored while the text sits in the bucket
- `--dry-run` and `--tenant=<slug>`; `chunkById(200)` so a backlog cannot exhaust
  memory or skip rows
- scheduled nightly at 03:30 in `routes/console.php`. **Nothing runs it on this
  Mac**: that needs `php artisan schedule:work`, or cron in production
- 6 tests in `tests/Feature/RetentionTest.php`: the bodies go and the score, task,
  tokens and timings stay; recent bodies are untouched; each tenant gets its own
  window; a dry run changes nothing; running twice is safe; reading a purged body
  returns null rather than failing. 65 backend tests in total
- VERIFIED against real MinIO, not just the fake disk: a body was written, read
  back as `this text must not survive retention`, purged, and confirmed gone from
  the bucket while `task_id`, tokens and duration remained

## Per-tenant encryption keys — DONE 2026-09-19 (D9, local half)

Each tenant now has its own 32-byte data key, stored wrapped in
`tenants.data_key`; bodies are AES-256-GCM under that key. Two defaults were taken
without asking, both reversible and recorded in D9:

- wrapping uses the application key locally, behind `BodyStore::wrap`/`unwrap` —
  the only two methods a KMS move touches
- bodies written before the change are left alone and stay readable under the
  application key until retention deletes them. The `AIULv2:` prefix on new
  ciphertext is what lets a body say which key it needs, so no column was added to
  `ai_interactions` and no re-encryption pass exists

Verified against real MinIO and Postgres: a legacy body and a new body were read
back correctly side by side, `tenants.data_key` unwraps to 32 bytes, and the
stored object contains none of the plaintext. 73 backend tests.

## Phase 8 — IN PROGRESS, without signing (2026-09-19)

The owner has no Apple Developer account yet and asked for Phase 8 without
signing. So everything up to the signature is built now, and signing is one
export away rather than a rewrite.

Plan, in order:

1. `scripts/build.sh` — universal binary (arm64 + x86_64 via `lipo`), version
   stamped with `-ldflags -X main.version`, output in `dist/`. `main.version`
   becomes a `var` so the linker can set it.
2. `scripts/package.sh` — `pkgbuild` component package with `/usr/local/bin/aiul`
   as its payload and a `postinstall` script. Signs ONLY when
   `AIUL_INSTALLER_IDENTITY` is set, so the same script produces the signed
   package later with no edit.
3. `packaging/scripts/postinstall` — provision a CA if none exists, then
   `aiul install --apply --yes`. `ca init` refuses to overwrite, so the check is
   `aiul ca info || aiul ca init`. Both run with `AIUL_STATE_DIR=/var/db/aiul`, so
   the CA is created where the worker will look rather than in root's home.
4. `docs/PACKAGING.md` — how to build and install the package, how to deploy it
   from an MDM, what to allow-list in an EDR, and the exact signing and
   notarization commands to run once there is an account.

What is deliberately NOT in this phase, and why:

- **No `.mobileconfig` carrying the CA.** Each device provisions its own CA, so a
  single profile cannot carry the certificate to trust. The production answer is
  D3's per-tenant root and per-device name-constrained intermediate, which is
  designed and not built. A profile written now would have an empty payload.
- **No signature, no notarization.** No account. Gatekeeper will therefore warn on
  a double-clicked package, and `installer` from the command line still works.
  Recorded as the one blocker for a real pilot.

### Phase 8 MILESTONE PASSED on this Mac, 2026-09-19

`sudo installer -pkg dist/aiul-0.8.0.pkg -target /` reported "The install was
successful", and `aiul status` afterwards showed the proxy listening, both jobs
loaded, the CA trusted, 4 of 4 network services pointing at 127.0.0.1:8899 and 9
environment variables written. `sudo aiul uninstall` put the Mac back.

Two bugs found by doing it, both fixed:

1. **The first attempt failed at the MDM gate.** `installer` does not pass its
   environment to package scripts, so `sudo AIUL_DEV_ALLOW_UNMANAGED=1 installer`
   had no effect. The override is now the root-owned marker file
   `/etc/aiul-dev-unmanaged`, which the kill switch removes (`dba660c`). The
   failure itself was clean: install rolled back and the Mac kept working.
2. **Uninstall removed the CA trust by file path, and the path was wrong.** The
   installed agent trusts the CA under `/var/db/aiul`, while `sudo aiul uninstall`
   resolved paths for the person who typed it and found their own CA, so
   `security` said "The specified item could not be found in the keychain". The
   delete-by-name step saved it, but the trust removal had been aimed at the wrong
   certificate. Removal now works from identity — every certificate under the
   common-name prefix, in a loop, since a Mac can hold two. `aiul status` reports
   the installed CA path too, rather than this user's.

Verified afterwards: no AIUL certificate in the System keychain at all.

## D3 — the device chain. DONE and VERIFIED ON REAL TRAFFIC 2026-09-19 (D14)

Installed from the 0.9.0 package and proven live:

```
$ echo | openssl s_client -connect api.openai.com:443 -proxy 127.0.0.1:8899 \
    | openssl x509 -noout -issuer
issuer=O=AI Usage Logger (development), CN=AIUL Dev Root Device - VWS18s-MacBook-Air.local
```

The certificate the client accepted was signed by the DEVICE certificate, not the
root. `aiul status` showed the proxy listening, both jobs loaded, the CA trusted
and 4 of 4 services proxied.

Two more reporting bugs fixed after that run: `aiul ca device` showed this user's
certificate rather than the running agent's, and the device directory was 0700 so
the certificate — which is public, and says which hosts the device may sign for —
could not be read by anyone but the service account. Directory is 0755 now, key
still 0600, and "cannot read it" no longer reports as "does not exist".

The owner deferred the Apple and AWS accounts to the end, so this is the next
piece of real work that needs neither. Only the root's home needs KMS; the chain
itself, the name constraints and the renewal can all be built and tested now, with
the dev root standing in for KMS exactly as `BodyStore::wrap` stands in for it in
D9.

Today every device trusts a self-signed root whose key sits on that device. If the
laptop is stolen, that key mints a certificate for any website in the world. D3
fixes that, and the fix is the single most important security property in the
product.

The shape, from D3:

```
root CA            key in KMS in production, the dev root for now
   │ signs         long-lived, one per tenant
device intermediate  key generated ON the device and never leaving it
   │ signs           SHORT-lived (7 days), NAME-CONSTRAINED to AI hosts
leaf certificates    minted on demand, 24 hours, as today
```

Plan, in order:

1. `internal/ca/intermediate.go` — generate a device key, have the root sign an
   intermediate for its PUBLIC key only, with:
   - `IsCA`, `MaxPathLen: 0` so it can sign leaves and never another CA
   - `PermittedDNSDomains` from the allow-list, marked CRITICAL. This is the
     control that matters: a correctly implemented client REJECTS a certificate
     from this intermediate for any name not on the list, so a stolen laptop
     cannot impersonate a bank even with the key in hand. It is enforced by the
     verifier, not by our code.
   - 7 days' validity, renewed when fewer than 2 days remain
2. Leaf minting moves behind a small issuer type, so a leaf can be signed by the
   root (development, `ca demo-server`) or by the intermediate (everything else),
   and the served chain becomes leaf → intermediate → root.
3. Storage under the state directory: `device/` holding the intermediate and its
   key at 0600, next to `dev-ca/`.
4. `aiul ca device` to provision, renew and inspect; `aiul run` renews on start
   and in the health loop, so an intermediate never expires under a running agent.
All five done. What was built, and what it was verified against, is D14 in
DECISIONS.md. On this Mac: `aiul ca device` shows the chain, and `openssl` on the
real certificate confirms `CA:TRUE, pathlen:0`, `Name Constraints: critical` with
19 permitted domains, and the key at mode 0600.

### The first packaged install of the chain FAILED, and why (2026-09-19)

`installer` reported "The upgrade failed". The worker log said:

```
cannot provision this device's signing certificate
err="this root cannot issue an intermediate: it was created with a path length of 0"
```

`/var/db/aiul/dev-ca` still held the PRE-D3 root, left there by earlier uninstalls
(they deliberately keep that directory so a spool is never destroyed unasked). The
postinstall asked "is a CA present?", found one, and correctly declined to
overwrite it — but that CA could not issue the device certificate, so the worker
refused to start. Install waited its 20 seconds, rolled back, and the Mac kept
working.

The fix is a new command, `aiul ca ensure`, which asks the question deployment
actually needs: keep a usable CA, create one when there is none, replace one that
cannot issue the intermediate — removing the old certificate from the keychain
first, since its key is about to be thrown away. The postinstall calls that
instead of `ca info || ca init`. Proven against the real pre-D3 root: `pathlen:0`
in, replaced, `pathlen:1` out, intermediate issued.

Also silenced `pkgbuild`'s four `write: Permission denied` lines. They are it
failing to copy `com.apple.provenance`, which macOS puts on every executable and
nobody can remove; the package is correct, verified with `pkgutil --expand-full`.
Only that exact line is filtered, and the exit status still decides.

Two things found while building it:

- the dev root carried `MaxPathLen 0` — "may sign leaves, no further CAs" — so it
  could not issue an intermediate at all, and every verifier would have rejected
  the chain. New roots carry `MaxPathLen 1`; `SignIntermediate` detects an old one
  and says how to fix it. The owner's root was regenerated (backed up to
  `dev-ca.pre-d3-backup`, trusted nowhere at the time).
- an existing test asserted the root must never issue a CA, which was correct
  before D3 and wrong after it. Now it asserts the real rule: one level below the
  root, none below the intermediate.

Tests, the load-bearing two first:
   - a leaf for `api.openai.com` from the intermediate VERIFIES against the root
   - a leaf for `evil.example.com` from the same intermediate is REJECTED by
     verification, with the name-constraint error. This is the whole point.
   - the intermediate cannot sign another CA
   - renewal triggers at the right time and not before

The allow-list lives in `internal/proxy` and the certificate code in `internal/ca`,
which must not import each other. The permitted domains are therefore passed in by
the caller, derived from the allow-list with any `*.` prefix stripped.

## Next — after D3

Phase 8 has no more work that can be done without an Apple Developer account. What
remains in it is signing, notarization and MDM delivery, all of which need the
account — `docs/PACKAGING.md` has the exact commands ready.

The owner decides what comes next:

- start the Apple Developer membership (a company account needs a D-U-N-S number,
  which takes longer than the membership; worth starting early)
- D3 — the production CA chain, which is what a real pilot needs more than a
  signature does
- D9's production half — `wrap`/`unwrap` in `BodyStore` become KMS calls
- D3 is the largest remaining piece of real engineering that does not need an
  account for its design, only for its deployment

Smaller things that could go first, none of them blocking: the production KMS half
of D9, brotli/zstd decoding, and the dev seed password (`password` on three
seeded users) which must not reach a pilot.

Worth carrying into Phase 8 and into the Linux and Windows ports: every bug found
on 2026-09-19 was invisible to the unit tests, because the tests run as the
developer, in directories the developer owns, in a process that inherited the
developer's environment. The installed agent runs as a service account with no
home, no inherited environment and no access to anyone's files. Anything that
reads a person's filesystem, or expects a variable, has to be checked on a real
installed run.

### DONE — how the privilege split was verified (2026-09-19)

The order used, so an interruption left the Mac working:

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
5. DONE. `aiul status`: "This Mac has no aiul settings applied." Verified by hand
   afterwards: no `aiul` process, no `/var/run/aiul-helper.sock`, no plist in
   `/Library/LaunchDaemons`, `dscl . -read /Users/_aiul` returns
   `eDSRecordNotFound`, `/etc/zshenv` is gone, and `curl https://example.com`
   works.

Left behind on purpose: `/var/db/aiul` (still `drwxr-x--- 448 448`, an owner that
no longer exists) holding the worker's CA copy and **two spooled events with a
real prompt in plaintext**. Remove with `sudo rm -rf /var/db/aiul`.

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

- ~~Prompt bodies use one application-wide encryption key~~ DONE 2026-09-19:
  per-tenant data keys, wrapped in `tenants.data_key`, AES-256-GCM bodies. What is
  left of D9 is the production half: `wrap`/`unwrap` in `BodyStore` become KMS
  calls. Bodies written before the change stay readable under the application key
  until retention deletes them.
- ~~Brotli and zstd response bodies are recorded as metadata only~~ FIXED
  2026-09-21 (D15). D12 had closed this on the evidence available then; two days
  later claude.ai began answering with zstd and the debug line said so, which is
  exactly the trigger D12 named. Both are decoded now.
- ~~Retention~~ DONE 2026-09-19: `aiul:purge-bodies`, scheduled nightly. Note that
  nothing runs the Laravel scheduler on this Mac, so it purges only when run by
  hand here.
- ~~Local dev seeds three users with the password "password"~~ FIXED 2026-09-19
  (D13): `DevUsersSeeder` refuses to run outside local/testing, generates a random
  password unless `AIUL_SEED_PASSWORD` is set, and the three existing accounts
  were rotated off `password`.

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

**Re-verified clean on 2026-09-19** after four installs (one from the package),
one kill switch and two uninstalls. One
new leftover that no earlier session had: `/var/db/aiul`, owned by uid 448 (the
deleted `_aiul`), holding the worker's CA copy and two spooled events whose prompt
text is in plaintext. `sudo rm -rf /var/db/aiul` removes it. Uninstall leaves it
deliberately, so a spool is never destroyed without being asked for.


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
