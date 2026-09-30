# Discovery — Aayatti PM on top of the AI Usage Logger

Date: 2026-09-30. Status: **approved 2026-09-30.** Decisions in section 0; the rest is the original discovery.

## 0. Decisions (2026-09-30)

- **Stack: Laravel module in `pm-tool/`, run by the existing `backend/` app. Not Next.js.**
  Nothing is live, so a rewrite was on the table. Laravel still wins: the owner writes
  PHP/Laravel; the logger's users, tenants, sessions and encrypted prompt text are
  already in this app (D18: one users list); a Next.js app would need a second login,
  a second users list, and would have to decrypt prompt bodies whose key lives in
  Laravel. `pm-tool/` holds all PM code (`src/`, `database/`, `resources/js/`,
  `tests/`); `backend/` only autoloads it (`Pm\\` namespace) and registers
  `Pm\\PmServiceProvider`. One app, one login, clearly separate code.
- **Outcome rating:** a rating, deferred. No column until it is designed.
- **Linking runs after the fact in the backend.** No agent change. "Start working"
  records a work period (`pm_work_periods`); a session that starts inside it links
  explicitly. Stamping at capture would need the agent to call the server on every
  prompt and fails offline; after-the-fact linking is also recomputable.
- **Managers see prompt text** (redacted copy, as the dashboard already shows).
  Raw, unredacted text stays behind the existing `can_view_raw_prompts` grant.
- **Linking layers:** explicit (1.0), task key in branch or prompt (0.9), only
  in-progress task (0.6), inbox. The LLM suggester is **not built** for now: it
  sends prompt text to a third party and guesses; add it only if the inbox is
  too busy in real use.


## 1. What the logger is

| Piece | Stack | Role |
|---|---|---|
| `agent/` | Go, one binary `aiul` | TLS-inspecting proxy on each Mac. Parses AI traffic, redacts secrets, spools events, posts them to the backend. |
| `backend/` | PHP 8.3, Laravel 13, Inertia + Vue 3, Tailwind 3, PostgreSQL (local: port 5433, db `aiul`), Horizon | Ingests events (`POST /api/aiul/events`), builds sessions, stores data, serves the manager dashboard (`/usage`). |

Multi-tenant: every row carries `tenant_id`. Roles today: `admin`, `member`
(plus a separate `can_view_raw_prompts` grant, admin only).

### How capture works

1. Agent sees a prompt/response pair for an allow-listed AI host and parses it
   (parsers: anthropic, openai, chatgpt_web, claude_web, claude_cowork,
   cloudcode/antigravity, copilot_web, cursor, gemini).
2. Redacts secrets on its copy, adds context: working dir `repo`, git `branch`,
   git `remote` (new, `github.com/org/repo`, credentials stripped).
3. Backend stores one `ai_interactions` row; prompt and answer **text** go to
   object storage, encrypted with a per-tenant key (`BodyStore`). The row keeps
   only object keys.
4. Backend assigns the row to an `ai_sessions` row: same device + tool + task,
   gap < `AIUL_SESSION_IDLE_MINUTES` (default 30). **Sessions already exist**;
   the prompt's "derive sessions" step is not needed.

### Fields per interaction (`ai_interactions`)

`tenant_id, device_id, user_id, ai_session_id, event_id, host, path, status,
tool, model, parser, kind (human|agent|utility), automated, account, task_id,
repo, branch, remote, prompt_object, answer_object, prompt_chars,
answer_chars, prompt_tokens, response_tokens, request_bytes, response_bytes,
duration_ms, streamed, redacted (jsonb), occurred_at, deleted_at`.

`ai_sessions`: `tenant_id, device_id, user_id, tool, task_id, repo, branch,
remote, started_at, ended_at, interaction_count`.

### What the local data shows (dev DB, 4 987 interactions, 494 sessions)

- Tools actually seen: `cli`, `claude-code`, `cursor`, `codex`, `chatgpt-web`,
  `claude-web`, `copilot-vscode`, `githubcopilotchat`, `copilot-web`,
  `antigravity`. Antigravity from the sketch is real.
- Token counts: present on ~93% of rows (missing mostly on web tools where the
  page does not report usage).
- Prompt text: present on every `human` prompt; missing on many `agent`
  (automated) steps, by design.
- `task_id`: **0%**. The 2026-09-22 branch-regex tagging found no keys: normal
  branch names carry none (D18).
- `remote`: 0% locally (column added today; agent not yet reinstalled).

## 2. What the PM tool needs that is missing

| Need | Status |
|---|---|
| Tasks, projects, sprints, task events | Missing. New tables. |
| Link session → task | Missing. `task_id` on sessions is a string never filled. New `task_ai_links` table (D18 calls it `task_attributions`). |
| Project ↔ code mapping | Missing. New `project_remotes` (project maps to 1..n git remotes). |
| Cost | Missing. Tokens exist; add a model price table in config, compute at read time. |
| Manager role | Missing. Roles are admin/member; add `manager`. |
| Latency | Have `duration_ms`. |
| Session title | Missing, not needed. |
| Outcome score | Missing. New `tasks.outcome_score`, set by a person. Note: automated prompt **quality** scoring was removed 2026-09-28 by the owner's call; this is different — a human rating per task. |
| Retention / redaction | Already exist (redaction at capture, retention on bodies). Reuse. |

## 3. Stack recommendation: build inside the existing Laravel app, not a new Next.js app

The owner already decided (D18, 2026-09-30): **the logger and the PM tool
share one users/tenants list, in the same Laravel app.** A separate Next.js +
Prisma app would mean a second auth system, a second copy of users, and a
cross-app read of encrypted prompt bodies (the key lives in Laravel). The
prompt's own rule is "reuse the logger's stack if reasonable" — it is.

- **Where code goes:** `backend/` (Laravel), under clear PM names:
  `app/Models/Pm/*`, `app/Http/Controllers/Pm/*`, `resources/js/Pages/Pm/*`,
  migrations prefixed `create_pm_*`. Existing aiul code is untouched except a
  menu link and the `manager` role.
- **`pm-tool/`** holds only docs (`DISCOVERY.md`, `METRICS.md`, later the
  README section). If you want `pm-tool/` to hold code, the only sane way is a
  separate Laravel package loaded by `backend/` — more ceremony, same result.
- Charts: no chart library today (plain SVG + Tailwind). Keep that; add one
  only if a chart needs it.
- Tests: PHPUnit, same as the 115 backend tests.

## 4. Data model (adapted)

Reused, not copied: `tenants` (= Organization), `users`, `ai_sessions`
(= AISession), `ai_interactions` (= AIInteraction). AITool = the existing
`tool` string plus a small config map for display name and vendor (no table:
the logger decides the tool names).

New tables (all with `tenant_id`):

- `pm_projects(id, name, key)` + `pm_project_remotes(project_id, remote)`
- `pm_sprints(id, project_id, name, start_date, end_date)`
- `pm_tasks(id, project_id, sprint_id?, number, title, description, status,
  assignee_id, estimate?, started_at, completed_at, outcome_score?)` — key =
  `project.key-number`, e.g. `AAY-42`.
- `pm_task_events(task_id, user_id, type, from, to, at)`
- `pm_active_tasks(user_id pk, task_id, since)` — the "Start working" button.
- `pm_branch_links(task_id, remote, branch)` — from "Copy branch name".
- `pm_task_ai_links(task_id, ai_session_id unique, method, confidence,
  confirmed_by?, confirmed_at?, dismissed)` — recomputable.
- `users.primary_ai_tool`: derived (most-used tool, last 30 days), not stored.
- Org settings: `tenants.settings` jsonb: `managers_see_content`,
  `content_retention_days`.

## 5. Linking engine — merged with D18

D18 (the owner's earlier design) and the build prompt overlap. Proposed ladder,
first rule that yields exactly one task wins:

| # | Method | Source | Confidence |
|---|---|---|---|
| 1 | `explicit` | User's active task when the session started | 1.0 |
| 2 | `convention` | Task key in branch name, or (remote, branch) in `pm_branch_links` | 0.9 |
| 3 | `time_window` | Exactly one `in_progress` task for the user in the project mapped to the session's remote | 0.6 |
| 4 | `ai_suggested` | LLM picks from the user's open tasks, capped 0.8 | ≤ 0.8 |
| — | inbox | Everything < 0.8 or unlinked: confirm / reassign / "Not task work" | confirm → 1.0 |

**Conflict to resolve (question 5 below):** D18 rejected matching prompt text
("privacy, and a guess"). The prompt wants (a) task keys found in prompt text
and (b) an LLM reading first prompts. I propose: key-in-prompt yes (exact
pattern, cheap, precise); LLM suggestion **off by default**, a tenant setting,
sends only metadata unless the setting says prompts may go.

Runs as a queued job after each session closes, and on demand when a task or
branch link changes (recompute past sessions).

## 6. Screens (Inertia/Vue, same shell as the dashboard)

Team Pulse, Task AI Trail (signature), Person, Tool & Cost, Board, Unlinked
inbox drawer — as specified. Team Pulse overlaps the existing Overview v4
(habit bar, "Who needs me"); plan: Team Pulse becomes the manager home and
reuses v4's attention cards, Overview stays as the adoption view.

## 7. Privacy

Already in place: redaction at capture, encrypted bodies, audited raw view.
New: managers see metadata by default; content only if
`managers_see_content` is on, and the member sees a notice. Members always see
their own. No ranking of people.

## 8. Seed

`PmDemoSeeder` (local only, like `DemoDataSeeder`): tenant "Aayatti",
people Parit (Claude), Saurabh (Antigravity), Pardeep (Claude),
Aakash (Claude), Mann (ChatGPT); project AAY; one sprint; tasks on every
status; Parit's AAY-1 trail on Mon 14 Sep 2026 10:34 → 14:15 with prompts
1.1–1.4 (explicit link); enough extra sessions for each linking method and
the inbox so no screen is empty.

## 9. Phases

As in the prompt, but Phase 1 needs no "read adapter": the tables are in the
same database. Each phase ends with verify commands and a stop.

## 10. Open questions

1. "1/10 R…" — task rating (0–10), rework count, or other? Default: 0–10 rating.
2. Stamp `task_id` at capture time (agent calls `GET /api/me/active-task`) or
   link after the fact only? Default: after the fact only (no agent change;
   the backend knows the active task at session start anyway).
3. Tools/tokens — answered above: 10 tools seen, tokens on ~93%, missing on
   some web tools. Cost is an estimate from tokens × config prices.
4. Manager access to content — default: metadata only, tenant toggle to allow,
   member notified.
5. D18 said no prompt-text matching. Allow task keys in prompts, and an
   opt-in LLM suggester? Default: keys yes, LLM off.
6. Code location: inside `backend/` (recommended) vs code in `pm-tool/`?
