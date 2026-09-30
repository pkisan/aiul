# Aayatti PM

A project management tool that knows how AI was used on each task. Normal PM
tools see tasks and status changes. This one also sees the AI sessions behind
them, because it reads what the AI Usage Logger (`aiul`) captures.

- **Board:** projects, sprints, tasks, and a five-column board. "Start working"
  marks the task you are on.
- **Task AI Trail:** on every task page. Every AI session behind the task, with
  its prompts numbered 1.1, 1.2, 2.1…
- **Team Pulse** (managers): four numbers, who may be stuck, and the team A–Z.
- **Tools & cost** (managers) and **My work** / person pages.
- **Inbox:** AI sessions the tool could not tie to a task for sure. People
  confirm them, pick the task, or mark them "not task work".

Every number is defined in [docs/METRICS.md](docs/METRICS.md). How and why it was
built this way: [docs/DISCOVERY.md](docs/DISCOVERY.md) and D18/D19 in
`../docs/DECISIONS.md`.

## How it fits with the logger

This is not a separate app. `pm-tool/` is a Laravel module that the logger's
backend (`../backend`) loads, so there is one app, one login and one list of
people:

```
Mac with aiul ──events──▶ backend/ (Laravel) ──▶ ai_sessions, ai_interactions
                              │
                              └── loads pm-tool/ ──▶ pm_* tables, /pm pages
```

The PM code reads the logger's tables. The logger never reads the PM tables.

| In `pm-tool/` | What it holds |
|---|---|
| `src/` | PHP, namespace `Pm\`: models, controllers, `Linking/Linker.php`, `Metrics/` |
| `database/` | migrations (`pm_*` tables) and `PmDemoSeeder` |
| `resources/js/` | Vue pages (`Pages/Pm/*`) and components |
| `routes/web.php` | every `/pm` route |
| `config/pm.php` | model prices, stuck-task thresholds |
| `tests/` | PHPUnit tests, run as the backend's "Pm" suite |

`backend/` loads the module in four places: `composer.json` (autoload),
`bootstrap/providers.php` (`Pm\PmServiceProvider`), `resources/js/app.js` and
`resources/views/app.blade.php` (which pick up `Pm/*` pages), and
`vite.config.js` and `tailwind.config.js` (which build them).

## Run it locally

Set up the backend first (`../docs/SETUP-MAC.md`: `docker compose up -d` for
Postgres, Redis and MinIO). Then, from `backend/`:

```sh
composer install          # also registers the Pm\ namespace
php artisan migrate       # creates the pm_* tables
npm install && npm run build
php artisan serve --port=8088
```

Demo data: tenant "Aayatti" with Parit, Saurabh, Pardeep, Aakash and Mann, and
project AAY. It includes Parit's AAY-1 trail from 14 Sep 2026 (10:34 AM → 2:15 PM,
prompts 1.1–1.4). Running it again rebuilds it. It refuses to run in production.

```sh
php artisan db:seed --class='Pm\Database\Seeders\PmDemoSeeder'
```

Sign in at http://127.0.0.1:8088 as `manager@aayatti.test` with the password it
prints. Managers land on Team Pulse; everyone else lands on the Board.

## Linking AI sessions to tasks

The linker gives each AI session one task, by the first rule that points at
exactly one task:

1. **Explicit (1.0):** the person pressed Start working on the task before the
   session began.
2. **Branch key (0.9):** the branch name contains the task key, e.g.
   `aay-42-cart`. "Copy branch name" on the task page gives one.
3. **Only task in progress (0.6):** the person had just one task started and not
   finished at the time. This is a suggestion that goes to the inbox.

Anything else goes to the inbox. A person's decision in the inbox is never
overwritten. The linker runs every 5 minutes, and at once when someone starts
or stops work, or changes a task's status or assignee:

```sh
php artisan schedule:list | grep pm:link   # scheduled
php artisan schedule:work                   # locally: runs the schedule in the foreground
php artisan pm:link --days=30               # relink the last 30 days now
```

On a server, the existing Laravel scheduler cron (`schedule:run` every minute)
also runs `pm:link`. Nothing else is needed.

## Cost

Cost is an estimate: tokens × list price per model, from `config/pm.php`. Claude
prices are set. Other vendors' prices are `null` on purpose (we have not checked
them), so they show as "no price" and count as $0. Add them there to include
them. Chat subscriptions (claude.ai, chatgpt.com) are not billed per token, so
for them the cost means "what this would cost on the API".

## Who sees what

- **Members** see the board, their own My work page and their own prompt text.
- **Managers and admins** also see Team Pulse, Tools & cost, anyone's person
  page, and prompt text in trails (owner's decision, 2026-09-30).
- **Managers only** create projects and sprints.
- Everything is confined to the signed-in person's tenant.

## Tests

```sh
cd ../backend
php artisan test --testsuite=Pm   # PM only
php artisan test                  # everything
```

## Deploying

`scripts/package-plesk.sh` (Plesk zip) and `compose.demo.yaml` (Docker) already
include `pm-tool/`. After deploying, run `php artisan migrate`.
