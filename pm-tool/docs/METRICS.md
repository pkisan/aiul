# Metrics — exact definitions

Every number on the PM insight screens, as computed in
`pm-tool/src/Metrics/Insights.php` and `Cost.php`. If one changes, change it
here too.

## Building blocks

- **Period.** The sprint running today (the first one found across projects),
  or the last 7 or 30 days (`?period=sprint|7|30`). Days are UTC dates.
- **Session.** An `ai_sessions` row from the logger: one person, one tool, no
  gap over 30 minutes. Sessions from devices not linked to a person are left
  out of every PM number.
- **Prompt.** An interaction with `kind = human`: something a person typed.
  Agent steps (`kind = agent|utility`) count for tokens and cost, never as
  prompts.
- **Link.** A `pm_task_ai_links` row: which task a session was for, how we know
  (`explicit`, `convention`, `time_window`, `manual`) and how sure (0–1).
- **Trusted link.** A link with a task that is either strong (confidence ≥ 0.8:
  explicit or branch key) or confirmed by a person in the inbox. Weak
  suggestions (0.6, "only task in progress") are not trusted until confirmed.
- **Not task work.** A link with no task, confirmed by a person.

## Team Pulse

| Metric | Definition |
|---|---|
| Done tasks that used AI | Tasks with `status = done` and `completed_at` in the period that have at least one trusted link, divided by all tasks done in the period. |
| Prompts per AI-assisted task | Prompts in the trusted-linked sessions of those AI-assisted done tasks, divided by the number of AI-assisted done tasks. The denominator excludes tasks done without AI, so the number describes AI work, not all work. |
| AI cost | Estimated cost of every interaction that happened in the period (see Cost). Also shows the share of tokens that have a price. |
| AI work linked to tasks | Sessions started in the period with a trusted link, divided by sessions started in the period minus those marked "not task work". This is the data-trust indicator: low means the other numbers see only part of the work. "Wait in the inbox" = sessions that are neither trusted-linked nor marked not task work. |
| Needs attention (prompt churn) | A task in progress with ≥ 10 prompts (`pm.churn_prompts`) in the last 48 hours (`pm.churn_hours`) in sessions linked to it (weak suggestions included), and no status change in those 48 hours. A stuck signal, not a verdict. |
| People table | Everyone in the tenant, A–Z (never ranked by AI use). Main AI tool = the tool with most sessions in the period. In progress = tasks assigned with `status = in_progress`. Sparkline = sessions per day in the period. |

## Task AI Trail

- Sessions: every session linked to the task (weak suggestions included,
  marked "suggested"), oldest first.
- Numbering: `session.prompt`, e.g. 1.1, 1.2, 2.1. Agent steps are shown as a
  count on the prompt they followed.
- Prompts / tokens / cost / AI time: over those sessions. AI time = sum of
  session lengths (first to last interaction).
- Prompt text: visible to the person whose session it is and to managers.

## Person

| Metric | Definition |
|---|---|
| Tasks done | Tasks assigned to the person with `completed_at` in the period. |
| AI sessions | Their sessions started in the period. |
| Linked to tasks | Their trusted-linked sessions divided by their sessions minus "not task work". |
| AI time | Sum of their session lengths in the period. |
| Split | Linked (trusted) · suggested (weak, unconfirmed) · not task work · not linked. |

## Tools & cost

| Metric | Definition |
|---|---|
| Cost (estimate) | Σ over interactions of `prompt_tokens × input price + response_tokens × output price`, prices per million tokens from `pm-tool/config/pm.php`, longest model-prefix match. Models without a price count as $0 and are listed. Subscription chat apps are not billed per token: for them it means "what this would cost on the API". |
| Cost per AI-assisted done task | Cost of the trusted-linked sessions of tasks done in the period, divided by the number of those tasks. |
| By tool and model | Per (tool, model): distinct sessions, prompts, tokens, cost, for interactions in the period. |

## Not built yet

- **Cycle time, AI vs not.** Needs more finished tasks than the demo has to
  mean anything; when added, label it correlation, not causation.
- **Outcome rating** (the sketch's "1/10"): deferred by the owner.
