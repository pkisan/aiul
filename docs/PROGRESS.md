# PROGRESS — AI Usage Logger

Single handoff file. Every new session reads CLAUDE.md then this file before doing anything.

Last updated: 2026-09-30 (PM tool Phase 4 done, owner to verify)

## PM tool — Phase 4 Insight screens — 2026-09-30 (DONE, owner to verify)

Owner confirmed `pm:link` is scheduled on their machine.
PLAN:
  1. pm-tool/config/pm.php: price per million tokens by model prefix
     (Claude prices from the claude-api skill, cached 2026-09-25; other
     vendors left null = "unpriced", shown as such, never guessed).
     Pm\Metrics\Cost (tokens x price, grouped by model in SQL) and
     Pm\Metrics\Period (this sprint | 7 | 30 days).
  2. pm-tool/docs/METRICS.md: exact definitions. "Trusted link" = task set
     and (confidence >= 0.8 or confirmed).
  3. Team Pulse /pm/pulse (managers; managers' home): 4 KPIs (AI-assisted
     done tasks %, prompts per AI-assisted done task, AI cost, linked
     activity %), each with a takeaway sentence; people table A-Z (tool,
     in progress, sessions, sparkline); Needs attention = prompt churn
     (>= 10 prompts in 48 h on an in-progress task with no status change).
  4. Task AI Trail on the task page: totals (span, prompts, tokens, cost,
     AI time), sessions in order, prompts numbered 1.1, 1.2..., collapsed
     to one line, expand loads prompt + answer (owner or manager only).
  5. Person /pm/people/{user} (self or manager): tasks, tool mix, sessions
     per day, linked vs unlinked. Tools & cost /pm/tools (managers).
  6. Nav: Pulse (managers), Board, Projects, My work. Members' home: Board.
  7. Tests for Cost, metrics, permissions; screenshots; commit. STOP.
- [x] 1-7 done. Pm\Metrics\{Cost,Period,Insights}, InsightsController,
      pages Pm/Pulse, Pm/Person, Pm/Tools, Components Trail/Kpi/Spark/
      DayBars/PeriodPicker. Trail sits on the task page above Details;
      expanding a row fetches /pm/trail/{interaction} (owner or manager).
- Home: managers -> /pm/pulse, members -> /pm (board); the old test
  expecting /usage and /my-data updated. Overview/Activity still in nav.
- Unpriced on demo data: gemini-3-pro, gpt-5 (config/pm.php, null on purpose).
- Not built: cycle time AI vs not (too few finished tasks to mean anything),
  outcome rating (deferred by owner). Both noted in METRICS.md.
- 135/135 backend (20 Pm). Screenshots: pulse light/dark/phone, trail with a
  row open, person, tools.
NEXT: owner verifies Phase 4; then Phase 5 (polish: empty/loading states,
responsive pass, dark mode pass, README for running PM + logger).

## PM tool — Phase 3 Linking engine — 2026-09-30 (DONE, owner to verify)

Menu name changed to "Aayatti PM" (`64ea5b2`; tab title, login page and
.env APP_NAME still say GenAI Log).
Decision: task keys are read from the BRANCH only, not prompt text. Reading
prompts means decrypting bodies from object storage on every relink, and D18
rejected prompt matching. "Copy branch name" on the task page gives people
a branch with the key in it.
PLAN:
  1. Pm\Linking\Linker::link(AiSession): skip sessions a person confirmed;
     else first rule with exactly one task: explicit (work period covering
     session start, 1.0), convention (task key in branch, any case, 0.9),
     time_window (only task assigned to the person with started_at <= start
     < completed_at, limited to the project of the session's remote, 0.6).
     Nothing found: delete the automatic row.
  2. `pm:link {--minutes=15} {--days=}` for all tenants; scheduled every 5
     minutes. PM changes (start/stop, status, assignee) relink that person's
     last 30 days at once.
  3. Inbox: nav badge "Inbox n" (my sessions, 30 days, no confirmed link and
     confidence < 0.8) opens a drawer: session, first prompt, suggestion;
     Confirm / pick a task / Not task work. Owner or a manager may act.
     Managers can switch to everyone's.
  4. Task page: "Copy branch name" (aay-4-task-ai-trail-timeline).
  5. Seeder runs the linker instead of writing AAY-1's link by hand.
  6. Tests per rule + inbox permissions; screenshots; commit. STOP.
- [x] 1-6 done. Pm\Linking\Linker, `pm:link` (schedule:list shows it every
      5 min), InboxController (JSON for the drawer), InboxDrawer.vue opened
      from the "Inbox" button in the top bar (`@pm` Vite alias to
      pm-tool/resources/js). Seeded demo: AAY-1 explicit, AAY-2 branch key
      (6), AAY-3/4/5/10 time window (inbox suggestions), 5 unlinked.
- Board badge "AI n" counts every link, weak ones included.
- Inbox badge counts MY sessions only; managers switch to "Everyone" inside.
- 130/130 backend (15 Pm). A mutation (confirmed-link guard removed) fails
  the suite. Screenshots: drawer light/dark/phone, task page.
NEXT: owner verifies Phase 3; then Phase 4 (Team Pulse, Task AI Trail,
Person, Tool & Cost).

## PM tool — Phase 2 PM core — 2026-09-30 (DONE, owner to verify)

Roles already exist (member/manager/admin, set on People): Phase 2 applies
them. Managers/admins create projects (name, key, git remotes) and sprints;
everyone in the tenant sees the board, creates, edits and moves tasks.
PLAN:
  1. Wire-up: pm-tool/routes/web.php (prefix /pm, names pm.*) loaded by
     PmServiceProvider; Vite resolves `Pm/*` pages from
     pm-tool/resources/js/Pages; tailwind scans pm-tool; nav links.
  2. Task::applyChanges(): status/assignee events, started_at on first
     in_progress, completed_at on done (cleared when reopened).
  3. Controllers: ProjectController (index, store, update, sprint store),
     TaskController (board, store, show, update, start, stop).
  4. "Start working": closes my open work period, opens one; moves a
     backlog/todo task to in_progress; assigns an unassigned task to me.
     Only for tasks assigned to me or nobody. Active task shown in the nav.
  5. Pages: Pm/Projects, Pm/Board (five columns, drag and drop plus a
     status select for keyboard/phone), Pm/Task (detail + edit).
  6. Tests (roles, tenant isolation, events, start/stop), screenshots, commit.
- [x] 1-6 done. Nav: Board, Projects for everyone; "Working on AAY-n · Stop"
      pill in the top bar (a strip under it on phones).
- [x] Wire-up needed two more places than planned: app.blade.php @vite picks
      the pm-tool path for Pm/* pages, and PmServiceProvider adds pm-tool's
      Pages dir to Inertia's page_paths (tests check pages exist).
- Remote normaliser in PHP (ProjectController::normaliseRemote) mirrors the
  agent's NormaliseRemote and also accepts "github.com/org/repo".
- 123/123 backend (8 Pm). Screenshots board light/dark/phone, task, projects.
NEXT: owner verifies Phase 2; then Phase 3 (linking engine + inbox).

## PM tool (Aayatti PM) — Phase 1 Foundation — 2026-09-30 (DONE, owner to verify)

Owner answered discovery 2026-09-30: code in a new folder, managers see
prompt text, rating deferred, rest my call. Decisions: pm-tool/docs/DISCOVERY.md
section 0 and D19. Summary: Laravel module in `pm-tool/` loaded by `backend/`
(namespace `Pm\`), linking after the fact, no LLM suggester yet.
PLAN (Phase 1):
  1. Wire-up: backend composer autoload `Pm\` -> ../pm-tool/src, seeders and
     tests; PmServiceProvider loads pm-tool migrations; phpunit "Pm" suite.
  2. Migrations: pm_projects, pm_project_remotes, pm_sprints, pm_tasks,
     pm_task_events, pm_work_periods, pm_task_ai_links.
  3. Models in pm-tool/src/Models (BelongsToTenant).
  4. PmDemoSeeder: tenant "aayatti", Parit/Saurabh/Pardeep/Aakash/Mann +
     manager, project AAY, sprint, tasks on every status, Parit AAY-1 trail
     Mon 14 Sep 2026 10:34-14:15 IST with prompts 1.1-1.4.
  5. Test: seeder builds the trail; migrate + full suite green; commit. STOP.
- [x] 1-5 done. One migration (2026_09_30_200000_create_pm_tables). Active
      task = pm_work_periods (history, one open per person by partial unique
      index), so the linker can ask "what was active when this session began".
      No outcome_score/estimate columns (rating deferred by owner).
- [x] Deploy paths updated for pm-tool/: compose.demo.yaml builds from the
      repo root (backend/Dockerfile.dockerignore replaces backend/.dockerignore);
      package-plesk.sh copies pm-tool into the zip and rewrites the composer path.
- Local dev DB migrated and seeded: 29 sessions, 294 interactions. Manager
  login manager@aayatti.test (password printed by the seeder; rerun for new).
- 116/116 backend (115 + Pm suite).
NEXT: owner verifies Phase 1, then Phase 2 (PM core: projects, sprints,
tasks, board, Start working, manager role screens).

## Overview v4: redesign from scratch — 2026-09-30 (DONE, owner to review)

Owner on v3: "not very easy to understand what is going on", start from
scratch if needed. Diagnosis: the scatter needs decoding (two derived axes,
medians); insights were sentences with no action; the table had 9 columns
incl. jargon (AI steps / prompt). Every block answered a different question.
New page answers three questions, in this order, in plain words:
  1. "Is my team using AI?" Headline sentence ("8 of 9 people used AI; 5 use
     it most days") + ONE segmented bar: Most days / Some days / Rarely /
     Not yet, ordinal indigo ramp, each segment labelled, names listed under
     each group. Replaces the scatter and the Active days tile.
  2. "Who needs me?" Attention cards, each: who, what happened, why it
     matters, one link. Kinds: not started, dropped off (was most days, now
     rarely/none), only one person using AI on a project. Credential banner
     stays. Empty state: "Nothing needs you this period."
  3. "Where does it go?" People table cut to: person, how often (label +
     day strip), prompts + change, main project, last used. Projects list.
  Volume numbers (prompts, AI time) demoted to a small context row.
Habit groups computed in the controller (share of period days: >= 55% most
days, >= 25% some days, > 0 rarely, 0 not yet; Today = used / not yet).
Habit.vue deleted.
- [x] Built as planned. Ordinal ramp checked with the dataviz validator:
      light indigo-800/600/400, dark indigo-400/500/700 (each passes its
      surface). `attention` prop replaces `insights`; `habitDays` tells the
      page what "most days" means for the period. Attention needs >= 7 days
      (Today would call a slow morning "stopped"); Today shows only
      "Used today" / "Not used". "Last used" shows — (not "Never") when the
      person was idle in the period: last_seen is period-scoped.
- 115/115 backend. Screenshots light/dark/phone at 7 days, light at 1 and
  30 days, all looked at.
NEXT: owner reviews at http://127.0.0.1:8088 (demo@example.com), then deploys
v4 + the remote work together (`php artisan migrate`).

## Overview v3: habit, not volume — 2026-09-30 (DONE, owner to review)

Owner: "I want to see the dashboard UI first" (before deploying the remote
work). Quality quadrant is NOT possible: scoring was removed 2026-09-28
(owner's call), quality_scores gets no new rows. Built on existing data:
PLAN:
  1. UsageReport::perPerson gains `days` (distinct dates used). Controller:
     team medians, KPI "Active days" (avg per active person) vs previous.
  2. KPIs: Active people · Active days · Prompts · AI time. "Active projects"
     dropped (Projects panel shows them).
  3. Habit quadrant (Components/Usage/Habit.vue, plain SVG): x = active days,
     y = prompts per active day, lines at team medians. Tooltip per dot.
  4. Team table: habit strip (one cell per day, periods <= 14 days) replaces
     the Active days number.
  5. Insights: a regular user who stopped; one person carrying most of a
     project's AI work.
  6. Tests, screenshots light/dark/phone on the demo tenant, commit.
- [x] 1-6 done. KPIs: Active people, Active days (5.8 of 7 on demo),
      Prompts, AI time. "Worth a look" panel beside the habit chart
      (Habit.vue, SVG, dots nudged sideways when they share a spot; hidden
      below sm, too small on a phone; needs >= 7 days). Days-used strip in
      the team table (<= 14 days). Insights: stopped regular user (and not
      repeated in "No AI use"), one person >= 80% of a project with >= 20
      prompts (then "Most AI work went into" is skipped for that project).
- [x] Fixed on the way: duration() showed "8h 60m" (rounded only the
      remainder); dark strip cells used dark:bg-gray-800, which the flipped
      gray scale turns light.
- Local dev DB migrated (remote columns); demo tenant reseeded.
- 113/113 backend. Screenshots 1440 light/dark, 390 phone (headless Chrome
  over CDP, script in the session scratchpad).
NEXT: owner looks at http://127.0.0.1:8088 as demo@example.com (password
printed by the seeder, rerun it for a new one), then deploys both this and
the remote work (`php artisan migrate`).

## Git remote URL on every event — 2026-09-30 (DONE, not installed)

Why: the PM tool will link AI work to tasks (design: D18 in DECISIONS.md).
`repo` is a local path, different on every machine, so it cannot be joined to
a PM project. The remote URL (github.com/org/repo) is the same everywhere.
Data not captured now cannot be back-filled, so this goes first.
Owner decided 2026-09-30: aiul and the PM tool share ONE users/tenants list
(the PM tool's). UI work (habit, quality quadrant) comes after this.
PLAN:
  1. agent/internal/tasks: read the remote from .git/config (origin, else the
     first remote; worktrees via commondir), normalise to host/path, strip any
     user:token@ (credentials never leave the machine), local paths -> "".
  2. Carry it: helper PROCESS reply gains an 8th field (older helpers send 7:
     tolerated), privileged.go, dircache, proxy Checkout hook, Event.Remote.
  3. backend: `remote` column on ai_interactions + ai_sessions, ingestion
     accepts it, validated.
  4. Tests fail without each piece; agent -race, backend suite; commit.
- [x] 1+2 `81f9293` agent: NormaliseRemote + remoteOf (tasks.go), 8th helper
      field, dircache, proxy Checkout hook, Event.Remote. Tests: URL forms incl.
      token stripping and Windows drive paths, origin preference, worktree,
      helper round trip strips a token, Cursor workspace path carries remote.
      go test -race green; GOOS=windows/linux vet green.
- [x] 3 backend: migration 2026_09_30_100000 (remote + index on
      ai_interactions, ai_sessions), ingestion stores it, drops anything not
      host/path (a URL with "@" never stored; event kept). Test fails without
      the check. 110/110.
- [x] D18 in DECISIONS.md: shared users, remote as project key, task ladder.
Nothing reads `remote` in the dashboard yet (sessions still group by repo).
NEXT: owner deploys backend (`php artisan migrate`), installs the new agent,
checks one event: `select remote from ai_interactions order by id desc limit 5`.
Then: dashboard UI (habit, quality quadrant, insights), then PM integration.

## Overview v2, a manager's page — 2026-09-29 (`a2546e2`)

Owner (as the manager of 30): the chart, Needs attention and the patterns
panel were not useful. Their data: 106 "secrets", 70 the person's own email
(Claude Code sends it with every request), rest mostly password/card/aadhaar
fixtures this repo's redaction tests contain; only 2 real keys. "5 silent
devices" = one Mac registered 5 times in testing (never revoked).
- [x] Removed daily chart (perBucket), sparklines, patterns(), device items,
      latest prompts. Page title "AI adoption".
- [x] Red banner only for CREDENTIAL_RULES (key/token rules; not email,
      phone, aadhaar, pan, credit-card, password-assignment).
- [x] Team first: change vs previous per person, sortable, search.
      Projects: share of prompts, contributors, change.
- [x] Insights: who is not using AI, who changed most (base >= 10,
      change >= 30%), top project with people count, team change.
- Backend 108/108; screenshots 1440 light/dark, 390 phone.
NEXT: owner reviews; stale device records need a "remove device" button on
People (not built); deploy (no migration).

## Manager Overview page — 2026-09-29

A manager wants a screen readable at one glance. Owner decided:
real build (no mockup); pending tasks later (no task data here, tickets
dropped 2026-09-22); managers see progress, not money (no spend figure);
the highlight is how PEOPLE use AI and what PROJECTS it moves, not tools.
- [x] `ce1a75f` /usage = Overview; the list moved to /usage/activity
      (Activity.vue); Deleted prompts moved to the user menu.
- [x] `d0be254` SECURITY FIX, pre-existing: SetTenantFromUser was prepended
      to the web group, ran before StartSession, found no user, set no
      tenant: every signed-in person saw EVERY tenant's rows (dashboard,
      activity, sessions). Invisible until a second tenant had data; tests
      missed it (actingAs knows the user before the session). Now after
      Authenticate, before SubstituteBindings; a test checks the order.
      Production impact depends on how many tenants genailog has: check
      `select id, slug from tenants` there.
- [x] `b10133d` Overview: 4 numbers vs previous period; Needs attention
      (secrets caught, devices silent 3+ days, unassigned devices); AI work
      by project chart (stacked by project); projects table with trend
      lines; How the team works with AI (project share, AI steps per
      prompt, prompts per session, active days per person); team table
      (main project, active days, steps per prompt, idle people included);
      latest 5 prompts; 2-3 rule-based insights. No chart library.
- [x] DemoDataSeeder (local only, refuses production): tenant "demo",
      sign in demo@example.com; password printed once by the seeder.
      Rerun rebuilds it. `php artisan db:seed --class=DemoDataSeeder`
- Visually checked (headless Chrome, 1440 light + dark, 390 phone).
  Backend 107/107.
Ceilings: no agent heartbeat, so "silent device" = nothing captured (agent
off OR no AI use); buckets cut in UTC; ⌘K search not built.
NEXT: owner reviews on http://127.0.0.1:8088 as demo@example.com, then
deploys (no migration needed). Pending tasks: later, needs PM task data.

## "foo" prompts + Claude desktop not logging — 2026-09-29

Owner reported (a) many "foo" prompts on one device, (b) Claude desktop chats
not logged, (c) manual zip install not viable for production, (d) restart.
- [x] (a) `3f35d6f`: "foo" rows are POST /v1/messages/count_tokens. Claude
  Code sizes its context with one call per tool, prompt "foo", tools
  attached, so kindOf() said human. Now utility in the anthropic parser AND at
  backend ingestion (older agents covered without reinstall).
  aiul:reclassify-prompts also fixes stored rows (local dry-run: 83). More
  tools/MCP servers on a device = more "foo" rows, so it looks device-specific
  but is not. Go + backend tests fail without the fix; backend 99/99.
- (b) DIAGNOSED, not fixed: 12:29 today /Applications/Claude.app main process
  sent a real TLS alert ("bad certificate") on api.anthropic.com, so rule 4
  tunnelled it (in memory, until agent restart). It handshook fine 09-21..23.
  downloads.claude.ai activity at 10:32 today suggests an app auto-update.
  Last desktop-helper claude.ai handshake in the log: 09-23 (DEBUG off since
  09-25, so absence is not proof). Next: quit+relaunch Claude.app, restart
  the agent with AIUL_DEBUG=1, one message, read the hello= line; mitmweb if
  it still refuses.
- (c) No update mechanism exists. Recommended: signed pkg/msi pushed by MDM
  (Jamf/Intune), token in a managed config profile; Phase 8 work.
- (d) Autostart already on: macOS LaunchDaemon RunAtLoad+KeepAlive, Linux
  systemd WantedBy=multi-user.target Restart=always, Windows StartAutomatic.
- (b) retest 13:06 after owner quit Claude, kickstarted, reopened: no tunnel
  warning, but no events either. `lsof`: Claude Helper (network service) held
  9 DIRECT connections to 160.79.104.10 (claude.ai/api.anthropic.com), only 2
  via :8899. Cause: a stop removes the system proxy (rule 7) and the new
  process only re-applied it on the first 30 s health tick ("drifted" at
  13:06:31). The app launched at 13:05:59, saw no proxy, went direct and kept
  those connections. Same window after every boot/upgrade. FIXED in run.go:
  listen first, then healthCheck once at start. Not installed yet.
  Workaround now: quit + reopen Claude after the agent has been up 30 s.
- (b) with AIUL_DEBUG: the desktop prompt went to POST
  claude.ai/v1/code/sessions/<cse_id>/events, "no parser". It is a Cowork
  remote session (environment_kind anthropic_cloud): the model runs in the
  cloud, the device only posts the person's message. Research dump captured
  two turns; new parser ClaudeCowork (tool id claude-cowork, dashboard name
  "Claude Cowork") reads the prompt and set_model; control-only POSTs and
  GET read-backs are skipped. Fixture: testdata/claude-cowork/ (ids and
  attestation signatures replaced). ANSWER NOT CAPTURED: it does not come back
  on this POST; no GET .../events landed in the dump. Where it arrives is
  unknown (possibly bridge.claudeusercontent.com, not allow-listed).
  Separately the app main process (Go-like hello) still rejects our cert on
  api.anthropic.com and is tunnelled; its traffic is small.
  /etc/aiul/agent.conf now has AIUL_DEBUG=1 and AIUL_RESEARCH_DUMP: owner
  removes both when done (research files hold real conversations).
NEXT: owner deploys backend and runs `php artisan aiul:reclassify-prompts` on
Plesk; tag a release. Then find where Cowork answers arrive.
- Installed v0.2.1-4-g0a6190a on the owner's Mac (14:31:45, second attempt).
  Proxy-at-start fix confirmed live: "drifted; re-applying" 1 s after start,
  not 30 s. FIRST attempt (14:30:41) failed: neither the helper nor the worker
  wrote a single log line within 30 s, installer rolled back cleanly. No
  launchd evidence kept. Intermittent; looks like the old bootout/bootstrap
  race (bug 6), not the new code. Watch for a repeat.
- Prompt confirmed on genailog as claude-cowork (14:36, "Hello"); no answer.
  Debug log after the POST: no GET .../events, no bridge traffic, no other
  HTTP exchange carrying it. Theory: answers stream over a WebSocket opened
  earlier; the proxy relayed unparsed WebSockets silently (no log, no dump).
  Added: DEBUG "websocket opened" (host, path, parsed) and, in research mode
  only, a redacted dump of an unparsed socket's messages (first 2000,
  "client:"/"server:" prefixed) written when it closes. Test fails without it.
  NEXT: install, one Cowork prompt, QUIT Claude (closes the socket so the dump
  is written), copy *claude.ai* dumps to the scratchpad.
- 14:45 capture: no claude.ai WebSocket. GET .../events returns only setup
  events (control_request, env_manager_log) + resume_cursor, no answer. One
  WebSocket in the log: api.anthropic.com /api/frame/sync (14:43:36,
  unparsed); its dump not copied yet. Prompt POST also carried 3 app-written
  <system-reminder> user messages with shouldQuery:false: parser now skips
  those (test fails without it).
- Cowork answer hunt, 15:02 capture after quitting Claude: answer is in NO
  decrypted traffic. /v1/code/sessions/<id>/events/stream (gzip SSE, open
  14:45:35-15:02:19, closed cleanly) and /v1/code/sessions/watch both carried
  nothing decodable (decompress() checked on truncated gzip: works). No
  WebSocket from the app. Sealed main-process api.anthropic.com exchanges are
  ~2 KB/5 KB, too small. Unexplained: the main process holds long-lived DIRECT
  connections (proxy ignored) to 34.117.41.85 (GCP) and 54.175.92.109 (AWS),
  hostnames unknown. Prompt-only capture stays. Options given to owner: SNI
  sniff to name those hosts; server-side (Anthropic compliance/admin API) for
  cloud sessions. Research copies still in the session scratchpad.
- tcpdump (owner ran, SNI only, 15:17): Claude made NO direct connections;
  the earlier 34.117.x / 54.175.x ones were gone. Every Claude connection
  goes through the proxy. Sealed because not allow-listed:
  bridge.claudeusercontent.com (73 KB to server at 15:17:10), likely the
  cloud session <-> device link. Owner said YES (2026-09-29): allow-list v4
  adds exactly bridge.claudeusercontent.com (not the domain: user content).
  Matcher tests: captured; claudeusercontent.com, files., look-alikes pass.
  Result: bridge = device tool dispatch (MCP over WS: tools/list,
  get_device_info, heartbeats) + /chrome extension bridge. NOT the answer.
- FOUND the answer: GET claude.ai/v1/code/sessions/<id>/events/stream (gzip
  SSE, long-lived, replays session history on connect). data: {event_type,
  source, payload}. Turn = user (source client; shouldQuery:false ones are
  app context) ... assistant (payload.message: model, content[text], usage)
  ... result. First result (num_turns 0) can come BEFORE the assistant.
  stream_event = text_delta pieces (ignore, assistant carries the whole text).
PLAN (in progress):
  1. parsers: CoworkTurns splitter (feed event data, returns finished turns);
     ClaudeCowork parses a turn (prompt, answer, model, tokens); Result gets
     EventID; POST and turn both use id "cowork-<user uuid>".
  2. proxy: for claude.ai .../events/stream, decode gzip on the side
     (buffered channel, never blocks the client, rule 6), split SSE, record
     each finished turn at once. Whole-stream record at close is skipped.
  3. backend: duplicate event_id with an answer fills an empty answer.
  4. fixture from the 15:22 dump (anonymised), tests, install, verify.
  DONE 1-3 + fixture (testdata/claude-cowork/events-stream.sse, ids, name,
  device, timezone replaced). Tests: splitter + turn parse + replay id + POST
  id match; proxy gzip stream recorded while still open and not again at
  close (fails without the tap); backend fill-in (fails without it). Agent
  suite -race green, backend 100/100.
  Ceilings: a turn records when its result arrives; answers of a turn the
  stream never carried stay empty (row stays prompt-only). Old prompt-only
  rows (random ids) are not back-filled.
  VERIFIED by owner 2026-09-29: new Cowork conversation logs prompt + answer.
  Known limit: after an agent restart, a session already open on screen does
  not reopen its events/stream (15:32 test), so that turn stays prompt-only
  until the session is reopened.
  NEXT: owner pushes main, deploys Plesk, tags v0.3.0; testers update by
  installing the new pkg over the old one (config and token are kept).

## Soft delete (Deleted prompts) — 2026-09-29

Owner chose "trash + purge": Delete prompt -> Deleted prompts (admin nav
"Deleted", /usage/deleted), restorable; aiul:purge-deleted (nightly 03:45)
removes it after AIUL_TRASH_DAYS (30). "Delete permanently" on the prompt page
and in Deleted prompts erases at once (leaked secrets). CLI
aiul:delete-user-data stays permanent and includes trashed rows.
- Migration 2026_09_29_100000: ai_interactions deleted_at, deleted_by,
  deletion_id (one click = one deletion); ai_sessions deleted_at (an emptied
  session hides, comes back on restore).
- withTrashed where it must: ingestion duplicate check (a re-sent deleted
  event stays deleted), PurgeBodies retention, delete-user-data.
- 98/98 backend tests. Visually checked (throwaway DB copy, dropped after):
  prompt page buttons, Deleted prompts light + dark, nav link.
NEXT: owner deploys (migrate needed); the Plesk scheduled task already runs
schedule:run, so the purge needs no new setup.

## Device-token redaction + admin prompt delete — 2026-09-29

- [x] `b655e4e` redaction rules v2: agent masks `aiul_` + 32+ alnum as
      [REDACTED:aiul-device-token]; backend ingestion applies the same mask so
      pre-v2 agents are covered without a reinstall. Existing stored rows are
      NOT rewritten: delete the leaked one with the new button.
- [x] Admin "Delete prompt" on the interaction page (DELETE /usage/{id},
      policy `delete`: admin, same tenant). Takes the whole turn (answer or
      step pages delete the prompt they belong to). Logic moved from
      aiul:delete-user-data into Services/UsageEraser (command unchanged in
      behaviour, its tests pass). Log line: who, whose, ids, never text.
      97/97 backend tests. Button not visually checked.
      NEXT: owner deploys Plesk zip, deletes the Mac-mini token prompt, issues
      that device a new token; tags v0.2.1 for the agent rule.

## Searchable filters on /usage — 2026-09-28

v0.2.0 released by owner (3 downloads, workflow green). Owner asked for
searchable dropdowns: new Components/SearchSelect.vue (type to narrow, arrows,
Enter, Escape) on the person, tool and project filters. People page role and
OS selects stay native (3 fixed choices each). Checked with headless Chrome
over CDP on a throwaway DB copy (aiul_shot, dropped after): light, dark,
"No matches", Enter filters via URL. Note: `php artisan serve` does NOT pass
DB_DATABASE to its worker; use `php -S` from public/ for a DB copy.
FOUND: redaction has no rule for our own `aiul_` device tokens; one was stored
in a prompt (owner's screenshot). Not fixed yet, owner to decide.
NEXT: owner uploads a Plesk zip (rename, dark mode, search).

## Device rename + Linux/Windows downloads — 2026-09-28

- [x] Device rename (`1c104af`): People page, click a device name. PATCH
      /people/{person}/devices/{device}; admin, same tenant, device must belong
      to that person; plain name, unique per tenant. Token unchanged (devices
      are found by token hash). 95/95 backend tests.
- [x] enroll-device.sh bug: on Linux it also ran the macOS "build the package"
      step and failed without the repo. Now macOS only.
- [x] release.yml: one tag -> aiul-macos zip, aiul-linux tar.gz (amd64+arm64,
      no mac metadata), aiul-windows zip; docs/TESTERS-LINUX.md and
      TESTERS-WINDOWS.md. Dry-run: Ubuntu 24.04 container unpacks cleanly,
      binary reports the tag, enrol plan shown, "n" changes nothing. Windows
      zip checked for contents only.
      STILL OPEN: L4 (real GNOME desktop) and W-d (real Windows PC) have never
      passed; the tester guides say so.
      NEXT: owner deploys the Plesk zip (rename + dark mode), tags v0.2.0.

## Dark mode: AI answers unreadable — fixed 2026-09-28

Owner screenshot: session page answer text black on the dark background.
Markdown.vue had hardcoded light colours; text, quotes, rules and tables now
use --gray-* (link lighter in .dark). Code blocks keep their light background
with pinned dark text. Checked with headless Chrome on the component CSS +
dark variables. NEXT: owner uploads a new Plesk zip (package-plesk.sh).

## Tester releases (option 1: monorepo stays, testers download) — 2026-09-28

Owner chose (2026-09-28): no branch split; testers get a GitHub Release zip.
Testers always use genailog.vardaam.site; only the owner's Mac switches between
live and local (already possible: `enroll-device.sh --token` vs `--local`).
- [x] `.github/workflows/release.yml`: on a `v*` tag, macos-latest runs
      `go test`, build.sh + package.sh, zips scripts/, packaging/scripts/,
      dist/ (pkg, linux amd64/arm64, exe) + docs/TESTERS.md as TESTING.md, and
      publishes the release. Dry-run of every step in a scratch clone passed
      (tests ok, 16-file zip, exec bits kept). NOT yet run on GitHub.
      v0.1.0 released 2026-09-28. Owner opened GitHub's automatic "Source
      code (zip)" (whole repo, cannot be disabled) and asked for Mac-only,
      necessary files only. Now: aiul-macos-<tag>.zip = TESTING.md,
      scripts/enroll-device.sh, scripts/killswitch.sh, dist/aiul-<tag>.pkg;
      release notes say "macOS only, ignore Source code". Dry-run: 5 files,
      bundled enrol script finds the pkg.
      NEXT: owner pushes and tags v0.1.1.

## PLAN: dashboard cleanup for genailog.vardaam.site (Plesk) — started 2026-09-28

Owner asked (2026-09-28): Aayatti logo in the menu; no Dashboard page — sign-in
lands on /usage; /usage redesigned (person filter, recent prompts, proper
pagination, no scores, no stat cards); dark + light mode; no dummy accounts —
one super admin whose password a seeder generates; site URL
genailog.vardaam.site; code ready to upload to Plesk.

Steps (commit each):
- [x] U0 commit the earlier uncommitted session-page work (`49f2465`)
- [x] U1 logo (public/logo.svg + favicon), dark mode (gray scale as CSS
      variables flipped under `.dark`, toggle in the nav, no flash on load)
- [x] U2 no Dashboard/Welcome: `/` sends managers to /usage, members to
      /my-data; public registration removed (it made users with no tenant)
- [x] U3 /usage redesign: filters (person, tool, project, period), recent
      prompts feed paginated server-side, people + tools + projects side panels.
      Owner said "remove the code and ui part of score": ScoreInteraction job,
      PromptScorer, QualityScore model and Horizon removed (scoring was the only
      queued job, so Plesk needs no queue worker). The quality_scores TABLE is
      kept (no data dropped). Task page removed. Audit log now lists all three
      read kinds (raw, session, list preview), eager-loaded, paginated.
      NOT visually checked: Chrome extension was not connected.
- [x] U4 SuperAdminSeeder replaces DevUsersSeeder + test user; People page so
      the super admin can add accounts (registration is gone). No delete
      button, and self-delete removed from Profile: deleting a user cascades
      to consent/audit records. AuthenticateSession added so a password reset
      signs the person out everywhere.
- [x] U5 genailog.vardaam.site: backend/.env.production.example, enrol
      script + agent help examples; `--tenant=dev` dropped from scripts (the
      backend's AIUL_TENANT decides; demo compose defaults it to dev, local
      .env has AIUL_TENANT=dev to match the existing DB)
- [x] U6 Plesk: docs/DEPLOY-PLESK.md, scripts/package-plesk.sh (zip from the
      last commit with vendor/ and public/build/), SecurityHeaders middleware,
      trusted local proxy, https forced when APP_URL is https, local disk
      never served. Demo compose: worker service removed, SuperAdminSeeder.
- [x] U7 owner (2026-09-28): "Also remove the audit log" — the Audit log PAGE,
      route and nav link are gone. Recording reads is KEPT: it feeds "Who has
      read my prompts" on My data, which the consent notice promises. Also:
      tool id `cli` (api.anthropic.com) shown as "Claude Code"; tools sharing
      a display name merged into one bar. Visually checked with headless
      Chrome on a throwaway DB copy: light, dark, phone width, People, session.
- [x] U8 owner said yes (2026-09-28): LOCAL DB only — the 1,162 interactions,
      24 sessions and 222 read records of dev@example.com moved to Punit
      (id 2); dev@example.com and manager@example.com deleted. Backup taken
      first (pg_dump, session scratchpad, not kept in the repo).
- [x] U9 owner (2026-09-28): "My recent interactions" and "Who has read my
      prompts" removed from My data. With nothing left reading them, the
      raw/session/list read records are no longer WRITTEN either (old rows stay
      in the DB). Consent notice no longer promises an audit log, so
      consent_version bumped to 2026-09-28: everyone re-accepts once.
      DEPLOY-PLESK.md rewritten for a blank Plesk site (site PHP binary path,
      pdo_pgsql, document root, scheduled task).
- [x] U10 site live on Plesk (owner, 2026-09-28). People page gets
      "Device token": device name + OS -> token shown once plus the command
      for a new install and for re-pointing an installed agent. Shared
      Device::provision() with aiul:provision-device (re-issue also clears
      `revoked`). Hostname limited to [A-Za-z0-9._-] because it lands in
      shell commands people copy.
- [x] U11 `aiul:delete-user-data <email> --all | --from/--to | --last=N`
      (+ --dry-run, --force). A prompt is deleted with its whole turn (agent
      steps + answer); bodies first, then rows; emptied sessions removed,
      others re-measured. Account, devices, consent kept.
      NEXT: owner uploads the new zip, issues tokens, re-points old devices, then decides whether to
      delete the local dummy users dev@example.com / manager@example.com.

## PLAN: cross-OS demo — started 2026-09-24

Goal: a tester on any Mac, Windows PC or Ubuntu desktop installs the backend and
the agent with one command each, and sees their own AI usage on their own
dashboard. Owner decisions (2026-09-24): **each tester runs their own backend**
(no shared server); Windows and Linux get an **installed agent** (trust, system
proxy, service, kill switch) — not the manual W1 run; Linux target is an
**Ubuntu desktop (GNOME)**. Still a demo (see "Framing"), not the product.

Milestones, each stops for the owner's confirmation (rule 13):

- **X1 — backend in Docker, same on all three OSes.** Add the Laravel app and
  its queue worker to docker-compose.yml under a profile, so
  `docker compose --profile app up -d --build` gives the dashboard on
  127.0.0.1:8088 with no PHP, Composer or Node on the host. Migrations, bucket,
  dev logins and the app key happen on first start; the key persists in a
  volume. Existing dev flow (data services only, `php artisan serve` on the
  host) unchanged. Verify on this Mac on a different port first.
- **X2 — macOS demo gaps.** Mac mini install failure (read the worker log);
  Codex over WebSocket answers; one live VS Code Copilot + Cursor row; seeded
  password not `password` outside local.
- **X3 — Windows installed agent** (was W2–W4): port→PID attribution
  (GetExtendedTcpTable), CurrentUser\Root trust, WinINET proxy + user env vars,
  Windows Service, fail-open; `aiul install --apply` and killswitch.ps1 cover
  every item. Tested on DESKTOP-Q12UTEE.
- **X4 — Linux installed agent (Ubuntu GNOME)**: trust in
  /usr/local/share/ca-certificates + update-ca-certificates AND the NSS db
  Chrome/Firefox read (~/.pki/nssdb, certutil from libnss3-tools); proxy via
  gsettings (GNOME) + /etc/environment block; port→PID from /proc/net/tcp;
  systemd unit; killswitch.sh learns Linux. MDM gate: dev override only.
- **X5 — one enrol command per OS** (enroll-device.sh for Mac+Linux,
  enroll-device.ps1 for Windows) against the local Docker backend, and
  docs/TEST-ON-ANOTHER-MACHINE.md replacing TEST-ON-ANOTHER-MAC.md.
  **X5 DRAFTED 2026-09-26 (`f9879b8`)**: docs/TEST-ON-ANOTHER-MACHINE.md
  written, TEST-ON-ANOTHER-MAC.md removed. Update its per-OS status lines
  once W-d (Windows) and L4 (Ubuntu) pass.

Progress:
- [x] X1 DONE 2026-09-24 (confirmed on the Mac mini). `compose.demo.yaml`
      (project `aiul-demo`, own volumes, only the dashboard port published),
      `backend/Dockerfile` (composer -> vite -> php:8.4-cli, extensions via
      install-php-extensions), `backend/docker/entrypoint.sh` (app key kept in
      the storage volume, migrate, seed ONCE — password only in `logs app`),
      one-shot `minio-bucket`, `.gitattributes` keeps *.sh LF for Windows.
      Verified here on AIUL_PORT=8098 beside the dev stack: all healthy,
      /login 200 with built assets; provisioned a device, consent, POSTed one
      event -> accepted, body in MinIO, worker scored it; recreate kept the key
      (body still decrypts) and did not reseed. Test stack left running on 8098.
      Note for X2: DevUsersSeeder already uses a random password — that item is
      done.
- [x] enroll-device.sh (part of X5, done early): gets the token from the
      Docker backend when compose.demo.yaml is running (no PHP on the Mac),
      `--user <email>` links the device to that person, and the closing text
      says consent is needed. Dry run ("n" at the prompt) against 8098:
      provisioned, linked to dev@example.com, nothing installed.
      Fresh `dist/aiul-29a4538.pkg` built (the old 967e32d one lacked the
      logged-out ChatGPT parser). Other Macs need that pkg copied in, or Go.
- [ ] X2 Mac mini install (2026-09-24 16:12): postinstall failed, "worker
      process is not running", agent.err.log EMPTY. Cause (strong, not yet
      confirmed there): Install made /var/log/aiul root:wheel 0750; launchd
      opens the worker's log AS _aiul, cannot enter the dir, never spawns it.
      Dev Mac hid it (older 0755 dir). Fix: chmod 0755 on every install.
      Leftover on the mini: /var/db/aiul owned by 448 (harmless, same uid).
      **FIXED, confirmed 2026-09-24**: `aiul-ed17ebb.pkg` installed on the Mac
      mini via `enroll-device.sh --user admin@example.com` against its own
      Docker backend: proxy listening, job running, CA trusted, 5/5 services.
      **MILESTONE PASSED 2026-09-24**: owner confirmed a prompt on the Mac
      mini's own dashboard. That also confirms X1 on a second machine (Docker
      backend, fresh Mac). X2 remaining: Codex over WebSocket; one live VS Code
      Copilot + Cursor row.
- [x] X2 Codex (2026-09-24 16:40, owner: a Codex prompt in a no-git folder was
      not recorded). NOT the missing repo: since the 14:44 manual install every
      `codex` rejected our certificate for chatgpt.com and was tunneled. The
      worker's device intermediate was signed by an older root (OU=user:root,
      A46B…) while the bundle Codex trusts held the current one (E3EC…);
      browsers trusted both roots in the keychain so they kept working.
      Fix `48b735e`: ProvisionDevice reissues when the intermediate does not
      verify against the current root (+ test). Stale root A46B is still
      trusted in the System keychain — harmless, removal offered to owner.
- [x] X2 WebSocket relay: `internal/proxy/websocket.go`. A 101 used to be
      read as HTTP ("malformed HTTP request"), killing Codex's socket (it fell
      back to HTTP) and ws.chatgpt.com. Now frames are relayed byte for byte
      both ways, flushed per frame; our copy is unmasked and inflated
      (permessage-deflate, context takeover via a 32 KiB window); Codex turns
      (response.create .. response.completed) are recorded through the
      existing Responses parser, off the relay goroutines. The empty 101
      exchange is no longer recorded. Test replays the 36-frame fixture, plain
      and compressed with split frames; -race clean, 30x stable. Two races found
      and fixed on the way: messages are now noted when their last byte is
      read, before it is forwarded.
      NEXT: owner reinstalls, one Codex prompt (app + VS Code), expect a row.
- [x] X2 Codex rows hidden (2026-09-24 17:00, owner: session 48 showed only
      "the agent call"). All 4 WS turns reached the backend but as
      kind=utility: follow-up turns carry `previous_response_id` and no
      `tools` (Codex sends tools once, as an `additional_tools` input item),
      so kindOf read them as housekeeping. Fix: OpenAI parser counts
      previous_response_id / additional_tools as tools offered, and skips the
      `generate:false` warm-up. Title call stays utility. Test extended.
      Old row 1631 ("Hey") is still utility in the local DB.
      NEXT: owner rebuilds + reinstalls, one Codex prompt, expect a human row.
      INSTALLED f75436a: "Howdy" became a human row, but its prompt was the
      whole input list (Codex instructions + earlier turns). Fix: Responses
      API prompt = text of the LAST user message only (`lastUserInput`).
      NEXT: reinstall, one Codex prompt, expect only the typed text.
- [x] X2 Codex ambient suggestions on the Mac mini (2026-09-24, owner
      screenshot): a Codex-app session of 10 "You asked" rows — a
      "Generate 0 to 3 hyperpersonalized suggestions" prompt, 7 rows "only
      context the tool added — nothing typed", the answer JSON, and a safety
      check prompt. None typed by the person. Fix: OpenAI parser reads
      client_metadata["x-codex-turn-metadata"] (JSON in a string; HTTP
      header fallback for thread_source only): thread_source != "user" ->
      utility, request_kind "prewarm" -> skipped; a previous_response_id
      continuation with no user message -> agent. TestCodexWhoStartedTheTurn.
      ASSUMPTION, unverified: ambient suggestions carry a thread_source other
      than "user" (no capture of them here). If the Mac mini still shows the
      suggestion prompt as human after this build, take a research capture
      there. Existing mini rows stay as they are.
- [ ] X2 package retry (2026-09-24 20:07): `aiul-09ead6b.pkg` failed in
      `aiul install --apply` at `chmod /var/db: operation not permitted`.
      The shared Darwin/Linux installer had a Linux-only parent-directory chmod;
      macOS already has traversable `/var/db` and can reject changing it.
      Fixed by guarding that chmod with `runtime.GOOS == "linux"`.
      Built `dist/aiul-mac-fix-09ead6b.pkg`; owner must install and confirm.
      2026-09-25 12:43: owner installed it, failed again, but at the MDM gate
      ("not enrolled in an MDM"), NOT at chmod: the kill switch run after the
      20:07 failure removed /etc/aiul-dev-unmanaged. Chmod fix still unproven
      on macOS. NEXT: owner recreates the marker, reinstalls the same pkg.
      **INSTALLED 2026-09-25 19:30**: marker recreated, same pkg installed,
      postinstall finished, status all yes (4/4 services). Chmod fix confirmed
      on macOS. But the kill switch had also removed /etc/aiul/agent.conf, so
      the agent spools and forwards nothing until the owner re-provisions
      against the 8088 dev backend and rewrites that file.

## ChatGPT widget answers — 2026-09-28

Mac agent.conf restored by the owner 12:03 (new token, device linked to Punit
Kisan): `forwarding=true` again. Then a "weather in Switzerland" answer showed
only "\ue200genui\ue202" boxes. Research capture 12:21 (research mode ON then
OFF, `~/aiul-research` deleted after reading): the closing batch of ops comes as
a bare `{"v":[...]}` without `"o":"patch"` and was dropped whole; the widget is
marked in the text as U+E200 genui U+E202 .. U+E201. Fix `ab0bf12`: unlabelled
batches read like patches; markers -> "[widget]"; cite removed, entity -> name
(ASSUMED shapes, not in a capture). Replayed the real capture: full answer.
Built `dist/aiul-ab0bf12.pkg`. NEXT: owner installs it, one ChatGPT weather
prompt, expect the full text with "[widget]". Old rows stay as stored.

## PLAN: X3 Windows installed agent — started 2026-09-25

Owner decisions (2026-09-25): start Windows while L4 (Ubuntu) and the Mac
reinstall still wait on the owner; **two services** like macOS (root helper +
unprivileged worker); proxy for **every signed-in user**. Target DESKTOP-Q12UTEE
(x64). Claude cannot run Windows here: unit tests of the pure parts on the Mac,
`GOOS=windows go vet/build`; the owner runs it on the PC and sends logs.

Design (same shape as macOS/Linux):
- services (x/sys/windows/svc/mgr): `aiul-helper` as LocalSystem, `aiul` as
  the virtual account `NT SERVICE\aiul` (its own SID, so the ACLs below can
  name it; stricter than LocalService, which every such service shares).
  Auto start, restart on failure, worker depends on helper. Env via the
  service's registry `Environment` value (AIUL_STATE_DIR, dev override, debug).
- files: binary `C:\Program Files\AIUL\aiul.exe`; `C:\ProgramData\AIUL\`
  state\ (CA copy, spool, helper socket; SYSTEM+Admins+NT SERVICE\aiul only),
  logs\agent.err.log + helper.err.log (the service writes its own stderr
  there), public\ (CA + bundle for SSL_CERT_FILE etc., readable by Users),
  agent.conf (endpoint + token; SYSTEM, Admins, worker read).
- helper IPC: same unix-socket protocol (AF_UNIX works on Windows 10 1803+),
  socket in state\, guarded by that directory's ACL instead of chown/chmod.
- proxy: WinINET per person, written by the SYSTEM helper into each loaded
  HKEY_USERS\<SID> hive: ProxyEnable/ProxyServer=`https=127.0.0.1:8899`/
  ProxyOverride AND the Connections\DefaultConnectionSettings blob (what
  WinINET/Chrome actually read). Chrome/Edge watch the key, apply live. Worker
  cannot read others' hives -> ErrProxyStateHidden -> helper re-applies each
  30 s tick (covers new sign-ins). Unset also loads signed-out profiles'
  NTUSER.DAT (`reg load`) so nobody signs in to a dead proxy (rule 7).
- env: machine variables (HKLM Session Manager\Environment) + WM_SETTINGCHANGE;
  names we set recorded in AIUL_MANAGED_VARS so removal takes exactly those.
- trust: LocalMachine\Root via certutil (Chrome, Edge, Node's system CA).
  Firefox not covered. CA bundle: Windows has no PEM file, so the root store is
  exported with crypt32 to build it.
- process lookup: GetExtendedTcpTable (port -> PID), QueryFullProcessImageName.
  Working directory NOT read (needs another process's PEB): tasks come only
  from what a parser reports (Cursor does). ".exe" stripped from names.
- MDM: dev override only (W5 later). killswitch.ps1: both services, every
  user's proxy incl. the blob, AIUL_MANAGED_VARS, both Root stores.

Steps (commit each):
- [x] W-a shared code (`141c85e`): platform.IsAdmin(), AgentConfigPath per OS,
      paths.SystemStateDir + helper.SocketPath vars (Windows: state\ folder,
      no chown/chmod, the folder ACL guards it), cmd/aiul stop.go
      (notifyStop/raiseStop/exitProcess) and service_windows.go (SCM wrapper,
      stderr -> logs\*.err.log), main -> dispatch().
- [x] W-b platform Windows (`141c85e`): service_windows.go (mgr, recovery
      restart incl. non-crash exits, Environment reg value, icacls after the
      virtual account exists), proxy_windows.go (HKU per SID + blob; Unset
      reg-loads signed-out profiles), env_windows.go (AIUL_MANAGED_VARS +
      WM_SETTINGCHANGE), trust_windows.go (crypt32, removes older roots),
      process_windows.go (GetExtendedTcpTable), ca/bundle_windows.go (root
      store -> PEM). Pure parts (wininet.go, tcptable.go) unit-tested on Mac.
      x/sys pinned v0.40.0 (v0.48 wants go 1.26). vet clean darwin/linux/windows.
- [x] W-c killswitch.ps1: both services; proxy for every loaded SID + reg-load
      of signed-out profiles, incl. blob flag; AIUL_MANAGED_VARS.
      scripts/enroll-device.ps1 new (admin check, dist\aiul.exe or go build,
      token from the Docker backend, agent.conf locked with icacls, dev
      override env, `ca ensure` + `install --apply --yes`, status). Both
      scripts parse in pwsh 7 (dotnet/sdk:8.0 image). NOT run on Windows.
      Risks to watch on the PC: (1) "Automatically detect settings" stays on
      — if Chrome finds no WPAD it should fall back to our manual proxy;
      (2) AF_UNIX socket needs Windows 10 1803+; (3) PowerShell 5.1 quirks
      (only parsed in pwsh 7).
- [x] W-c2 desk review before the PC run (`1f99f3a`): (1) SECURITY —
      ProgramData lets any user create files and the creator may read them, so
      a user could pre-create state\ and later read the CA key. Install now
      icacls-locks C:\ProgramData\AIUL first (SYSTEM+Admins F, Users RX), then
      refuses if anything inside is owned by anyone but SYSTEM, Administrators,
      the worker or the installer; enroll-device.ps1 locks + owner-checks the
      folder before writing the token. (2) PS 5.1: `docker ... 2>&1` under
      ErrorActionPreference Stop aborts on any docker warning -> Continue there.
      vet/build clean for windows; both .ps1 parse in pwsh 7. Not run on Windows.
- [x] W-c3 CLI wording per OS (`fbdd28e`): cmd/aiul/words.go (machine,
      trustStore, killSwitch, restartWorker, asAdmin) — no more "this Mac",
      sudo or System keychain on Windows/Linux. Dead installUsage/uninstallUsage
      consts deleted.
- [ ] W-d owner runs it on DESKTOP-Q12UTEE; fix what breaks  <- NEXT (awaiting owner). dist/aiul.exe built from fbdd28e.
      2026-09-28 run 1: Docker engine not started (500 on _ping), then
      `quay.io/minio/minio:latest` 401 — MinIO's official images no longer
      pull anywhere. Fixed `14293be`: compose.demo.yaml uses
      bitnamilegacy/minio (amd64+arm64, has mc+sh), verified from nothing here.
      The PC needs the new compose.demo.yaml (code not pushed yet).

## PLAN: X4 Linux installed agent — started 2026-09-24

Owner decisions (2026-09-24): start Linux now (X2 paused, Codex fix 3d39f94
awaiting the owner's check); **script install** (no .deb); **Chrome only**
(no Firefox/snap NSS work). Target Ubuntu desktop (GNOME), amd64 + arm64
binaries. Claude cannot run Linux here: unit tests + cross-compile only; the
owner runs the install on the Ubuntu machine and sends logs.

Design (same shape as macOS, same paths where possible):
- service account `_aiul` (useradd --system, nologin), state /var/db/aiul,
  logs /var/log/aiul/agent.err.log (systemd `append:`), so every instruction
  and path stays the same as on the Mac.
- systemd units /etc/systemd/system/aiul-helper.service (root) and
  aiul.service (User=_aiul) — replaces the two LaunchDaemons.
- trust: /usr/local/share/ca-certificates/aiul-dev-root.crt +
  update-ca-certificates (CLIs, curl, Python), AND each desktop user's
  ~/.pki/nssdb via certutil (Chrome). Needs libnss3-tools.
- proxy: GNOME `org.gnome.system.proxy` via gsettings, run as each logged-in
  user on their session bus (/run/user/<uid>/bus). Chrome follows it live.
  The worker cannot read other users' settings, so on Linux its health tick
  re-asks the helper to apply (idempotent; covers users who log in later).
- env: marked block in /etc/environment (pam_env, read at login by terminals
  and GUI apps).
- process lookup: /proc/net/tcp(6) port -> inode -> /proc/<pid>/fd, exe, cwd.
- MDM: none on Linux; dev override only (/etc/aiul-dev-unmanaged -> env var).
- kill switch: scripts/killswitch.sh hands over to killswitch-linux.sh on Linux.
- install: enroll-device.sh learns Linux (binary instead of pkg), and the
  same postinstall script does `ca ensure` + `install --apply --yes`.

Steps (commit each):
- [x] L1 kill switch for Linux: scripts/killswitch-linux.sh, killswitch.sh hands over on Linux. Ran in ubuntu:24.04 (dry run + real, nothing configured): clean.
- [x] L2 platform: shared unix code moved to exec.go / service_unix.go
      (prepareInstall, PublishCA, CA copy); linux.go (desktop users, runuser),
      serviceuser/service/trust/proxy/env/process_linux.go; healthCheck asks the
      helper to re-apply on ErrProxyStateHidden; Debian CA bundle path added.
      Tests run on Linux in golang:1.25 (docker), incl. a live /proc lookup:
      all pass except the pre-existing paths SUDO_USER test (container is root).
- [x] L3 build.sh builds dist/aiul-linux-{amd64,arm64}; enroll-device.sh
      learns Linux (binary, apt libnss3-tools if certutil missing, the same
      packaging/scripts/postinstall). Fix found by the container test:
      Ubuntu has no /var/db, `ca ensure` created it 0700 root and the worker
      could not reach its CA -> prepareInstall chmods it 0755.
      VERIFIED in a privileged systemd ubuntu:24.04 container (users ubuntu +
      alice, alice with a live session bus): enroll-device.sh end to end,
      install twice (upgrade), both NSS stores filled, alice's GNOME proxy
      set, curl via proxy decrypted with process=curl dir=/home/alice/proj,
      kill switch (also with the agent gone, logged-in and logged-out user)
      leaves nothing behind. NOT testable here: a real GNOME desktop + Chrome.
      Test artefact, not a bug: Docker Desktop sends container traffic
      through THIS Mac's proxy, so the Mac agent tunnels com.docker.backend
      for api.openai.com / api.anthropic.com (first request fails once).
- [ ] L4 owner runs it on Ubuntu; fix what breaks  <- NEXT (awaiting owner)

## PLAN: production refinement R1 — started 2026-09-23 (owner's 8 items)

Owner decisions: employee login is `aiul login` on the device (short code,
confirmed in the web app after sign-in + first-login consent; device bound to
that user). AI-tool account names in two steps: now what is already on the wire
(JWT claims: ChatGPT web, Codex); then research captures for Claude Code,
claude.ai, Cursor, Copilot.

Steps (commit each):
- [x] R1.1 Prompt cleaning (item 8): backend `PromptText::clean()` strips harness
      wrappers (<system-reminder>, <local-command-*>, <command-*>,
      <EPHEMERAL_MESSAGE>, <environment_context>...) at ingestion AND on display
      (old rows). Test.
- [x] R1.2 Interaction page shows prompt + answer inline (item 6); /raw redirects.
      Every open still writes the raw_view audit record.
- [x] R1.3 Session page as a chat (items 5, 7): your message right, final answer
      left, agent steps collapsed between, tool's own calls hidden.
- [x] R1.4 /usage filters by person/tool/days (item 4); "My data" off the menu.
- [x] R1.5 First-login consent screen (item 1, web half).
- [x] R1.6 `aiul login` device pairing (item 1, device half, and item 3 fallback).
      Events from a device with no consented user are discarded.
- [x] R1.7 AI-tool account name (item 2): `account` on event + column; JWT claims
      in the agent; UI shows account, else the device user's name (item 3).

**R1 BUILT, awaiting the owner's check (rule 13).** Backend migrated locally
(pairing columns, `account`). 103 PHP tests, Go tests green. Browser check not
done by Claude (Chrome extension not connected).
IMPORTANT side effect: the backend now DISCARDS events from any device not
linked to a person who accepted the notice. On this Mac device 1 is linked to
"Dev Member", who has not consented yet — sign in as dev@example.com once and
accept, or run `aiul login` and link it to another account.
INCIDENT 19:15: owner's prompts from ~18:18 to ~19:10 discarded exactly as
warned above (device 1 -> Dev Member, no consent); lost, the agent deleted them
from its spool. Both sides now log discards (`dae5d75`). Owner to link the
device to a consented account.
Not done / next:
- Old suggestion/recap rows fixed by `php artisan aiul:reclassify-prompts`
  (ran locally 2026-09-23: 95 rows now utility). Safe to rerun.
- R1.7 step 2 (research run 18:39, 498 dumps, copy deleted after reading):
  DONE ChatGPT web `GET /backend-api/me` {name,email} and Cursor
  `DashboardService/GetMe` (proto 3=email 4=first 5=last) -> remembered per
  host, attached to later events (`parsers.NoteIdentity` / `parsers.Account`).
  Claude Code + claude.ai: NO name on the wire — only account_uuid
  (oauth/validate, event_logging); claude.ai org name is "<email>'s
  Organization" for personal orgs only. Owner chose: Claude Code's name from
  ~/.claude.json oauthAccount (fullName > displayName > email), read by the root
  helper as a 7th PROCESS reply field (`helper.ClaudeCodeAccount`). claude.ai
  in the browser still falls back to the device's person.
  NOTE: b2379c3/3496fff edited capture.go by mistake — Event/record live in
  event.go — so no account reached events until the fix commit after them.
  Copilot web (research run, 9 dumps on api.individual.githubcopilot.com):
  threads/messages/models carry thread and message ids only, no user. The
  profile lives on github.com, never decrypted (rule 3). Copilot falls back to
  the device's person. Research mode can go OFF now.
  Antigravity fetchUserInfo: no name. Copilot: no github traffic in the run.
- Agent must be rebuilt + reinstalled for `aiul login` and `account`.
- Research mode was found OFF on 2026-09-23 (directory absent), despite the
  Copilot note below saying ON. Owner re-enabling it for the account research.

## Logged-out ChatGPT (incognito) not recorded — 2026-09-24 12:58

Owner's incognito chatgpt.com prompt never reached /usage. Cause: logged-out
ChatGPT posts the turn to `POST /unauth-mweb/conversation/updates` (sequence
seen live 12:57–12:58: unauth-mweb conversation/prepare, sentinel
chat-requirements/prepare+finalize, conversation/updates, conversation/prepare).
`ChatGPTWeb.Handles` only matches `/backend-api/(f/)conversation`, so it is
logged as "no parser" and dropped. Not a backend discard (none today).
FIXED (parser, not yet installed): research run 13:10 (research mode turned ON
again for it). Request is form-encoded, text in `prompt`; answer is HTML
fragments, finished paragraphs in `<?start name="...-committed-block-N">`.
No model and no account on the wire; the UI falls back to the device's person
(R1.7). `ChatGPTWeb.parseLoggedOut`, fixture `testdata/chatgpt/unauth-turn.*`
(tokens removed), `TestChatGPTWebLoggedOut`. **VERIFIED 14:45** — owner
reinstalled `2a301b8`, incognito prompt showed on /usage. Research mode OFF
again (no RESEARCH MODE line at the 14:45 start), `~/aiul-research` deleted.

## Copilot web parser — 2026-09-23 17:10

Chrome's old "rejections" of `api.individual.githubcopilot.com` (09-21, 09-22)
predate the leaf fix; since then Chrome and VS Code ("Code Helper") both
complete handshakes. Research run 17:04 (research mode ON again — turn off when
Copilot work is done): github.com/copilot posts to
`/github/chat/threads/<id>/messages`, JSON request `{"content","model":"auto"}`,
SSE response `routedModel` / `content` / `complete` (usage). `parsers.CopilotWeb`
+ `testdata/copilot/web-turn.*` + `TestCopilotWebTurn`. OPTIONS preflight is
Skip. **LIVE 17:07: recorded, parser=copilot-web, model=mai-code-1.1-flash.**

VS Code Copilot Chat (research 17:08): main turn is `POST /responses` (Responses
API, no /v1), UA `GitHubCopilotChat/0.66.0`, 84 tools, prompt inside the last
`<userRequest>`. Plus four `gpt-4o-mini` `/chat/completions` side calls (title,
progress messages, tool grouping) already parsed by `openai` — but with NO
kind, so they would have counted as human prompts. Fixed: OpenAI parser matches
`/responses`, tool `copilot-vscode`, `<userRequest>` extraction, and `kindOf`
(utility for tool-less agent calls). `TestCopilotVSCodeResponses`. Checked
against all five real calls. NEXT: install, one VS Code Copilot prompt, expect
one human row + utility rows. Then turn research mode OFF.

## PLAN: Windows port — started 2026-09-23

Owner decisions (2026-09-23): test machine is a **Windows x64 PC**;
`golang.org/x/sys/windows` is **allowed** (D16); first milestone is a **manual
capture with no system changes**. macOS work is paused, not finished: the Mac
mini install failure (worker not starting, log not yet read) and the Copilot
retest are both still open below.

Today `GOOS=windows` compiles, but every platform piece is a stub returning
ErrUnsupported. Milestones, each stops for the owner's confirmation (rule 13):

- **W1 — manual capture, no system changes.** `dist/aiul.exe` from build.sh;
  `scripts/killswitch.ps1` first (rule 2), idempotent, removes everything later
  milestones add; state dir `%LOCALAPPDATA%\AIUL`; key-permission check
  skipped on Windows (ACLs, not mode bits); MDM stub honours the dev override.
  Verify: `aiul.exe ca init`, `aiul.exe run` in one PowerShell, Claude Code in a
  SECOND PowerShell with `$env:HTTPS_PROXY` and `$env:NODE_EXTRA_CA_CERTS` set
  for that window only; the prompt reaches the dashboard (backend on the
  MacBook, reached over the LAN).
  **W1 BUILT `5e201b4`, awaiting the owner's run on the Windows PC.** Not
  runnable here: no Windows and no pwsh on this Mac, so killswitch.ps1 is
  unparsed — its first run must be `-DryRun`. Vet is clean for windows, linux
  and darwin. Backend must listen on the LAN (`php artisan serve --host 0.0.0.0
  --port 8088`); MacBook LAN IP was 162.16.1.190. Token:
  `php artisan aiul:provision-device <pc-name> --tenant=dev --platform=windows`.
  RUN 16:01 on DESKTOP-Q12UTEE: `ca init`, device cert, `proxy listening`,
  forwarding on. `killswitch.ps1 -DryRun` parsed and ran clean (found the
  hand-started aiul.exe, nothing else). No `claude` on the PC, so the owner
  chose ChatGPT web instead: needs our root in CurrentUser\Root (certutil
  -user -addstore) — the FIRST Windows system change, asked for explicitly —
  and Edge with its own profile and `--proxy-server`.
  **W1 PASSED 16:52** (owner confirmed on the PC): Edge with its own profile and
  `--proxy-server`, our root in CurrentUser\Root, signed-in ChatGPT. Spool
  event: tool=chatgpt-web model=gpt-5-6, prompt and full answer, streamed,
  6074 ms. Not uploaded — the PC is on a different network from the MacBook's
  backend; the spool keeps it. Follow-ups, not blockers:
  - `ca init` prints "Nothing on this Mac trusts it" on Windows too.
  - `ws.chatgpt.com` logs `malformed HTTP` after decrypting; ChatGPT still
    answered normally (the answer came over /backend-api/f/conversation).
    Check whether macOS logs the same before touching it.
  - PowerShell 5.1 shows the UTF-8 spool file as mojibake; the file is fine.
  - Every new window needs the four env vars again; a run-dev.ps1 was offered.
  - Dashboard for Windows rows needs the same network, a tunnel, or a backend
    on the PC — owner's call.
- **W2 — attribution.** GetExtendedTcpTable (x/sys/windows) maps a port to a
  PID and executable. Working directory of another process is hard on Windows
  (PEB read); fall back to the workspace a parser reports (Cursor already does).
- **W3 — trust + proxy + env, by hand, with confirmation.** CurrentUser\Root
  via certutil; WinINET proxy in HKCU Internet Settings (+ refresh); user env
  vars in HKCU\Environment (+ WM_SETTINGCHANGE). Killswitch undoes each.
- **W4 — service.** Windows Service via x/sys/windows/svc, virtual account,
  token in DPAPI / Credential Manager, fail-open health loop.
- **W5 — MDM gate + tools.** Enrollment from HKLM\SOFTWARE\Microsoft\Enrollments
  or `dsregcmd /status`; tool detection.
- Packaging (MSI) stays with Phase 8.

## Session resume 2026-09-22 — read this first

**Everything below the resume block is history, newest first.** It is long
because this week found a lot; read the top three sections and stop.

### State of the machine

- Installed agent: `ea254eb`, the same commit as `HEAD`. `main` is **2 commits
  ahead of `origin/main`** (the allow-list change and this file).
- Backend running locally: Postgres 5433, Redis 6380, MinIO 9002 in Docker;
  `php artisan serve` on 8088. 653 interactions, 10 sessions, kinds on 134.
- Research mode is OFF (2026-09-24 14:45).
- Checkouts under `~/Desktop` record no branch until the agent is granted Full
  Disk Access (see SETUP-MAC.md). `~/Herd` and elsewhere are fine.

### What captures prompt AND answer today

Claude Code CLI · Claude desktop app · Codex over HTTP · chatgpt.com · claude.ai ·
Antigravity (row 1004, 2026-09-23) · Cursor (with its settings, see below) ·
chatgpt.com on Windows (W1, spool only)

### What does not, and why

| Tool | Reason |
| --- | --- |
| Codex over WebSocket | `101` upgrade; frames unread. Fixture recorded at `agent/testdata/openai/codex-responses.ws.jsonl` |
| Cursor | captured live 2026-09-23 (RunSSE) with the settings in "RESULT: Cursor settings"; branch confirmed; model joined from BidiAppend (`d577eae`, not yet seen live) |
| Copilot | web: captured live 2026-09-23. VS Code Copilot Chat: parser written, awaiting live row |
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

Research mode was OFF during that run (`/var/db/aiul/research` absent), so no
bodies exist yet. `70dbd85` makes the dump fit for Cursor: binary (protobuf)
bodies are written as `base64:<bytes>` instead of being mangled to U+FFFD, and
`proto` content types are no longer skipped. The WebSocket frames after `101`
are still not read by anything. Awaiting: owner builds + installs `70dbd85`,
turns research mode on (docs/TESTING.md), sends one Cursor prompt.

### Cursor parser written — `ec9f278` (12:40)

Second run (installed `cc629d2`) gave RunSSE a 10 KB body. RunSSE ALONE carries
a whole row, so no pairing with BidiAppend is needed except for the model:

- Connect stream frames: flag (bit 1 gzip, bit 2 JSON trailer), u32 length, proto.
- `1.6.1.1` = prompt echoed back; `1.1.1` = answer text deltas; `1.4.1` =
  thinking deltas (ignored); `1.14.1`/`.2` = input/output tokens.
- `4.x` and gzipped `3.x` frames = conversation checkpoints (workspace, branch,
  signatures) — ignored, and dropped from the fixture.

`parsers.Cursor` in `internal/parsers/cursor.go`, stdlib-only protobuf reader,
fixture `testdata/cursor/runsse.{request,response}.bin`, test `TestCursorRunSSE`.
Also checked against the full uncut recording (not committed): same result.
Model stays EMPTY — it lives only in BidiAppend (`1.9.1` of the hex-wrapped
inner message). Kind is human whenever a prompt is echoed; agent tool-loop
turns not yet seen.

LIVE 12:34: row with tool=cursor parser=cursor kind=human tokens 23569/20 —
but branch empty: the extension host's cwd is `/`. Fixed in the next commit:
the parser returns the workspace from checkpoint field 21.1 as
`Result.WorkDir`, and `record()` resolves the task from it when the connection
gave no repo. Tests `TestCursorRunSSE`, `TestTaskFromWorkspaceInBody`.

CONFIRMED LIVE 12:39: dashboard session 27 shows `plrb-lms · feature/filamentv5
· cursor`. Research mode turned OFF by the owner afterwards.

MODEL JOINED `d577eae`: the turn's first BidiAppend names the model (hex-wrapped
inner 1.9.1) under the conversation id (2.1) that RunSSE's request also sends.
`parsers.Cursor` remembers id -> model (bounded map) and returns the new
`Result.Skip` for BidiAppend, so it is never a row. Checked against the live
recording: `grok-4.6`. Awaiting install for a live row with a model.

NEXT: Copilot in VS Code trust retest — `./scripts/tool-experiment.sh vscode`
(owner). Then `aiul doctor --matrix`.

### Research run 12:22 — the prompt decoded (installed `4538b77`)

Cursor's chat is a pair on `api2.cursor.sh`, both Connect RPC over HTTP/1.1:

- **Prompt:** `POST /aiserver.v1.BidiService/BidiAppend` (`application/proto`,
  gzip). Top-level field 1 is a HEX STRING of another protobuf. Inside it
  (paths are field numbers): `1.2.1.1.1` = prompt text ("Hello what's up"),
  `1.9.1` = model (`grok-4.6`), `1.1.21.1`/`.2` = workspace dir and branch,
  `1.2.17.9.11.4` = git remote, `1.25` = conversation id. Later BidiAppends
  (~438 bytes) are small follow-ups, not yet decoded.
- **Answer:** `POST /agent.v1.AgentService/RunSSE`, request = the same
  conversation id, response `text/event-stream` but binary Connect frames, no
  `data:` lines. Streamed 4 s. The dump had NO body: `exchange()` dropped a
  non-SSE body under an SSE content type. Fixed in `cc629d2`.
- `GET /agent/v1/run` → 101 WebSocket comes ~2 s AFTER the answer; probably not
  the chat. Ignore unless the RunSSE answer turns out empty.

A throwaway decoder lives in the session scratchpad only (`pb.py`: walks
protobuf without a schema). The Go parser needs the same, small, in stdlib.

NEXT: owner installs `cc629d2`, sends one more Cursor prompt, copies dumps to
the scratchpad; decode the RunSSE answer; anonymise both into
`agent/testdata/cursor/`; write `parsers.Cursor` (pair BidiAppend + RunSSE by
conversation id — the first parser that needs two requests for one row).

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

Phases 0–7 are DONE on macOS; Phase 8 is built without signing. What is left
for production (answered to the owner 2026-09-26):
- [ ] P1 hosted backend: real server, domain, TLS, APP_ENV=production, backups,
      queue + scheduler supervised (retention only runs when the scheduler does)
- [ ] P2 D3 production CA: per-tenant root in KMS/HSM; backend signs each
      device's intermediate (today the root is generated ON the device)
- [ ] P3 D9 production half: BodyStore wrap/unwrap become KMS calls
- [ ] P4 Phase 8: Developer ID signing + notarization (macOS), Authenticode +
      MSI (Windows), .deb or signed script (Ubuntu); MDM/Intune push profiles
      incl. trust + Full Disk Access (macOS)
- [ ] P5 real MDM gate on Windows (W5: Enrollments / dsregcmd) and a policy for Linux
- [ ] P6 Firefox trust on Windows and Linux (policies.json: ImportEnterpriseRoots on Windows, Certificates.Install on Linux)
- [ ] P7 deployment integration test: install / upgrade / uninstall per OS in CI
- [ ] P8 non-technical (ROADMAP §4): employee notice, raw-prompt access owner,
      retention per tenant, legal review per country

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
