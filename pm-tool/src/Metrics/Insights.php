<?php

namespace Pm\Metrics;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\User;
use App\Services\BodyStore;
use App\Services\PromptText;
use App\Services\UsageReport;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Support\Collection;
use Pm\Models\Task;
use Pm\Models\TaskAiLink;
use Pm\Models\TaskEvent;

/**
 * The numbers behind the insight screens. Every definition is written out in
 * pm-tool/docs/METRICS.md; keep the two in step. Runs in a web request, so
 * the tenant scope on each model confines every query.
 */
class Insights
{
    /** Links we count as fact: a task, and either strong (>= 0.8) or confirmed by a person. */
    public static function trusted(): Builder
    {
        return TaskAiLink::query()->whereNotNull('task_id')
            ->where(fn ($q) => $q->where('confidence', '>=', 0.8)->orWhereNotNull('confirmed_by'));
    }

    // ---------------------------------------------------------------- Team Pulse

    public function pulse(Period $p): array
    {
        $done = Task::where('status', 'done')->whereBetween('completed_at', [$p->from, $p->to])->pluck('id');
        $doneLinks = self::trusted()->whereIn('task_id', $done)->get(['task_id', 'ai_session_id']);
        $aiTasks = $doneLinks->pluck('task_id')->unique()->count();
        $prompts = AiInteraction::whereIn('ai_session_id', $doneLinks->pluck('ai_session_id'))->where('kind', 'human')->count();

        // Which tool touched the most AI-assisted done tasks.
        $topTool = AiSession::whereIn('ai_sessions.id', $doneLinks->pluck('ai_session_id'))
            ->join('pm_task_ai_links as l', 'l.ai_session_id', '=', 'ai_sessions.id')
            ->groupBy('tool')->selectRaw('tool, count(distinct l.task_id) as tasks')
            ->orderByDesc('tasks')->toBase()->first();

        $sessions = AiSession::whereNotNull('user_id')->whereBetween('started_at', [$p->from, $p->to]);
        $total = (clone $sessions)->count();
        $ids = (clone $sessions)->select('id');
        $linked = self::trusted()->whereIn('ai_session_id', $ids)->count();
        $notWork = TaskAiLink::whereNull('task_id')->whereNotNull('confirmed_by')->whereIn('ai_session_id', $ids)->count();
        $waiting = $total - $linked - $notWork;

        $cost = Cost::of(AiInteraction::whereBetween('occurred_at', [$p->from, $p->to]));

        return [
            'kpis' => [
                'assisted' => ['done' => $done->count(), 'ai' => $aiTasks, 'pct' => self::pct($aiTasks, $done->count()),
                    'tool' => $topTool?->tool, 'toolTasks' => (int) ($topTool?->tasks ?? 0)],
                'prompts' => ['perTask' => $aiTasks ? round($prompts / $aiTasks) : null, 'tasks' => $aiTasks],
                'cost' => $cost + ['pricedPct' => self::pct($cost['priced_tokens'], $cost['tokens'])],
                'linked' => ['linked' => $linked, 'total' => $total - $notWork, 'pct' => self::pct($linked, $total - $notWork), 'waiting' => $waiting],
            ],
            'people' => $this->people($p),
            'attention' => $this->stuck(),
        ];
    }

    /** Everyone in the tenant, A-Z: never ranked by AI use. */
    private function people(Period $p): Collection
    {
        $inPeriod = AiSession::whereNotNull('user_id')->whereBetween('started_at', [$p->from, $p->to]);
        $tools = (clone $inPeriod)->groupBy('user_id', 'tool')->selectRaw('user_id, tool, count(*) n')->toBase()->get()->groupBy('user_id');
        $daily = (clone $inPeriod)->groupBy('user_id', 'd')->selectRaw('user_id, date(started_at) d, count(*) n')->toBase()->get()->groupBy('user_id');
        $open = Task::where('status', 'in_progress')->whereNotNull('assignee_id')->groupBy('assignee_id')->selectRaw('assignee_id, count(*) n')->pluck('n', 'assignee_id');
        $days = $p->days();

        return User::where('tenant_id', auth()->user()->tenant_id)->orderBy('name')->get(['id', 'name', 'role'])
            ->map(function (User $u) use ($tools, $daily, $open, $days) {
                $perDay = collect($daily[$u->id] ?? [])->pluck('n', 'd');

                return [
                    'id' => $u->id, 'name' => $u->name,
                    'tool' => collect($tools[$u->id] ?? [])->sortByDesc('n')->first()?->tool,
                    'inProgress' => (int) ($open[$u->id] ?? 0),
                    'sessions' => (int) $perDay->sum(),
                    'spark' => array_map(fn ($d) => (int) ($perDay[$d] ?? 0), $days),
                ];
            });
    }

    /**
     * Prompt churn: in-progress tasks with many prompts in the last N hours and
     * no status change in that time. Suggested links count too: the point is to
     * notice someone going round in circles, not to prove it.
     */
    public function stuck(): Collection
    {
        $since = now()->subHours(config('pm.churn_hours'));
        $tasks = Task::with(['project:id,key', 'assignee:id,name'])->where('status', 'in_progress')->get()->keyBy('id');
        $moved = TaskEvent::whereIn('task_id', $tasks->keys())->where('field', 'status')->where('occurred_at', '>=', $since)->pluck('task_id');

        return AiInteraction::join('pm_task_ai_links as l', 'l.ai_session_id', '=', 'ai_interactions.ai_session_id')
            ->whereIn('l.task_id', $tasks->keys()->diff($moved))
            ->where('ai_interactions.kind', 'human')->where('ai_interactions.occurred_at', '>=', $since)
            ->groupBy('l.task_id')->selectRaw('l.task_id, count(*) n')
            ->havingRaw('count(*) >= ?', [config('pm.churn_prompts')])
            ->orderByDesc('n')->toBase()->get()
            ->map(fn ($r) => [
                'id' => (int) $r->task_id, 'key' => $tasks[$r->task_id]->key, 'title' => $tasks[$r->task_id]->title,
                'assignee' => $tasks[$r->task_id]->assignee?->name, 'prompts' => (int) $r->n,
                'hours' => (int) config('pm.churn_hours'),
                'since' => $tasks[$r->task_id]->started_at,
            ]);
    }

    // ------------------------------------------------------------ Task AI Trail

    /**
     * Every AI session linked to the task, in order, each prompt numbered
     * "session.prompt" (1.1, 1.2, 2.1). Prompt text only for people allowed to
     * read it: the person whose session it is, and managers.
     */
    public function trail(Task $task, User $viewer, BodyStore $bodies): array
    {
        $links = TaskAiLink::where('task_id', $task->id)->get()->keyBy('ai_session_id');
        $sessions = AiSession::with('user:id,name')->whereIn('id', $links->keys())->orderBy('started_at')->get();
        $rows = AiInteraction::whereIn('ai_session_id', $sessions->pluck('id'))->orderBy('occurred_at')
            ->get(['id', 'ai_session_id', 'kind', 'model', 'prompt_tokens', 'response_tokens', 'duration_ms', 'occurred_at', 'prompt_object'])
            ->groupBy('ai_session_id');

        $previews = 60; // ponytail: body fetches per page load; lazy-load previews if long trails get slow
        $out = $sessions->values()->map(function (AiSession $s, int $i) use ($links, $rows, $viewer, $bodies, &$previews) {
            $canRead = $s->user_id === $viewer->id || $viewer->isManager();
            $turns = [];
            $steps = 0;
            foreach ($rows[$s->id] ?? [] as $r) {
                if ($r->kind !== 'human') {
                    $steps++;
                    if ($turns) {
                        $turns[count($turns) - 1]['steps']++;
                    }

                    continue;
                }
                $turns[] = [
                    'id' => $r->id, 'n' => ($i + 1).'.'.(count($turns) + 1), 'at' => $r->occurred_at,
                    'tokens' => $r->prompt_tokens + $r->response_tokens, 'steps' => 0,
                    'preview' => $canRead && $previews-- > 0
                        ? UsageReport::preview(PromptText::clean($bodies->get($r->prompt_object)), 140)
                        : null,
                ];
            }
            $link = $links[$s->id];

            return [
                'id' => $s->id, 'n' => $i + 1, 'tool' => $s->tool, 'person' => $s->user?->name,
                'models' => collect($rows[$s->id] ?? [])->pluck('model')->filter()->unique()->values(),
                'started_at' => $s->started_at, 'ended_at' => $s->ended_at,
                'method' => $link->method, 'confidence' => $link->confidence, 'confirmed' => (bool) $link->confirmed_by,
                'canRead' => $canRead, 'turns' => $turns, 'steps' => $steps,
            ];
        });

        $all = AiInteraction::whereIn('ai_session_id', $sessions->pluck('id'));

        return [
            'sessions' => $out,
            'totals' => [
                'from' => $sessions->first()?->started_at,
                'to' => $sessions->max('ended_at'),
                'prompts' => (clone $all)->where('kind', 'human')->count(),
                'cost' => Cost::of($all),
                'seconds' => (int) $sessions->sum(fn (AiSession $s) => $s->durationSeconds()),
                'suggested' => $out->where('confirmed', false)->where('confidence', '<', 0.8)->count(),
            ],
        ];
    }

    // ------------------------------------------------------------------- Person

    public function person(User $u, Period $p): array
    {
        $sessions = AiSession::where('user_id', $u->id)->whereBetween('started_at', [$p->from, $p->to]);
        $ids = (clone $sessions)->select('id');
        $perDay = (clone $sessions)->groupBy('d')->selectRaw('date(started_at) d, count(*) n')->pluck('n', 'd');
        $linked = self::trusted()->whereIn('ai_session_id', $ids)->count();
        $suggested = TaskAiLink::whereNotNull('task_id')->where('confidence', '<', 0.8)->whereNull('confirmed_by')->whereIn('ai_session_id', $ids)->count();
        $notWork = TaskAiLink::whereNull('task_id')->whereNotNull('confirmed_by')->whereIn('ai_session_id', $ids)->count();
        $total = (clone $sessions)->count();

        $tasks = Task::with('project:id,key')->withCount('aiLinks as ai_sessions')->where('assignee_id', $u->id)
            ->where(fn ($q) => $q->where('status', '!=', 'done')->orWhereBetween('completed_at', [$p->from, $p->to]))
            ->orderByRaw("case status when 'in_progress' then 0 when 'in_review' then 1 when 'todo' then 2 when 'backlog' then 3 else 4 end")
            ->orderBy('number')->get();

        return [
            'kpis' => [
                'done' => $tasks->where('status', 'done')->count(),
                'sessions' => $total,
                'linkedPct' => self::pct($linked, $total - $notWork),
                'seconds' => (int) (clone $sessions)->get()->sum(fn (AiSession $s) => $s->durationSeconds()),
            ],
            'split' => ['linked' => $linked, 'suggested' => $suggested, 'notWork' => $notWork, 'unlinked' => $total - $linked - $suggested - $notWork],
            'tools' => (clone $sessions)->groupBy('tool')->selectRaw('tool, count(*) n')->orderByDesc('n')->toBase()->get(),
            'days' => array_map(fn ($d) => ['day' => $d, 'n' => (int) ($perDay[$d] ?? 0)], $p->days()),
            'tasks' => $tasks->map(fn (Task $t) => ['id' => $t->id, 'key' => $t->key, 'title' => $t->title, 'status' => $t->status, 'ai_sessions' => $t->ai_sessions]),
        ];
    }

    // ------------------------------------------------------------- Tools & cost

    public function tools(Period $p): array
    {
        $inPeriod = AiInteraction::whereBetween('occurred_at', [$p->from, $p->to]);
        $rows = (clone $inPeriod)->groupBy('tool', 'model')
            ->selectRaw("tool, model, count(distinct ai_session_id) sessions, sum(case when kind = 'human' then 1 else 0 end) prompts, sum(prompt_tokens) input, sum(response_tokens) output")
            ->toBase()->get()
            ->map(function ($r) {
                $price = Cost::price($r->model);

                return [
                    'tool' => $r->tool, 'model' => $r->model, 'sessions' => (int) $r->sessions, 'prompts' => (int) $r->prompts,
                    'tokens' => (int) ($r->input + $r->output),
                    'usd' => $price ? round(($r->input * $price[0] + $r->output * $price[1]) / 1_000_000, 2) : null,
                ];
            })
            ->sortByDesc(fn ($r) => [$r['usd'] ?? -1, $r['tokens']])->values();

        $daily = (clone $inPeriod)->groupBy('d', 'model')
            ->selectRaw('date(occurred_at) d, model, sum(prompt_tokens) input, sum(response_tokens) output')->toBase()->get()
            ->groupBy('d')->map(fn ($models) => round($models->sum(function ($r) {
                $price = Cost::price($r->model);

                return $price ? ($r->input * $price[0] + $r->output * $price[1]) / 1_000_000 : 0;
            }), 2));

        $done = Task::where('status', 'done')->whereBetween('completed_at', [$p->from, $p->to])->pluck('id');
        $doneLinks = self::trusted()->whereIn('task_id', $done)->get(['task_id', 'ai_session_id']);
        $aiTasks = $doneLinks->pluck('task_id')->unique()->count();
        $doneCost = Cost::of(AiInteraction::whereIn('ai_session_id', $doneLinks->pluck('ai_session_id')));
        $cost = Cost::of($inPeriod);

        return [
            'kpis' => [
                'cost' => $cost + ['pricedPct' => self::pct($cost['priced_tokens'], $cost['tokens'])],
                'perTask' => $aiTasks ? round($doneCost['usd'] / $aiTasks, 2) : null,
                'aiTasks' => $aiTasks,
            ],
            'rows' => $rows,
            'unpriced' => $rows->whereNull('usd')->pluck('model')->filter()->unique()->values(),
            'days' => array_map(fn ($d) => ['day' => $d, 'usd' => (float) ($daily[$d] ?? 0)], $p->days()),
        ];
    }

    private static function pct(int|float $part, int|float $whole): ?int
    {
        return $whole > 0 ? (int) round(100 * $part / $whole) : null;
    }
}
