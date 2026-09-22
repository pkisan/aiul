<?php

namespace App\Services;

use App\Models\AiInteraction;
use App\Models\AiSession;
use Illuminate\Pagination\LengthAwarePaginator;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;

/**
 * The numbers behind the dashboard.
 *
 * Every query here runs under the tenant global scope, so none of them mentions
 * tenant_id. Read that as the isolation working, not as a missing filter.
 */
class UsageReport
{
    public function __construct(private readonly int $days = 30) {}

    private function since(): Carbon
    {
        return now()->subDays($this->days);
    }

    /**
     * Usage per task: how many prompts, how much AI time, the average score.
     *
     * Untagged work is NOT hidden — it comes back as its own row, because an
     * "untagged" bucket that quietly disappears is how a dashboard starts lying.
     */
    public function perTask(): array
    {
        $interactions = AiInteraction::query()
            ->where('occurred_at', '>=', $this->since())
            ->select([
                'task_id',
                DB::raw('count(*) as interaction_count'),
                DB::raw('count(*) filter (where automated = false) as human_prompts'),
                DB::raw('count(*) filter (where automated = true) as automated_followups'),
                DB::raw('sum(prompt_tokens + response_tokens) as tokens'),
                DB::raw('max(occurred_at) as last_seen'),
            ])
            ->groupBy('task_id')
            ->get()
            ->keyBy(fn ($row) => $row->task_id ?? '');

        $time = $this->secondsPerTask();
        $scores = $this->averageScorePerTask();

        return $interactions->map(function ($row) use ($time, $scores) {
            $key = $row->task_id ?? '';

            return [
                'task_id' => $row->task_id,
                'untagged' => blank($row->task_id),
                'interactions' => (int) $row->interaction_count,
                'human_prompts' => (int) $row->human_prompts,
                'automated_followups' => (int) $row->automated_followups,
                'tokens' => (int) $row->tokens,
                'ai_seconds' => (int) ($time[$key] ?? 0),
                'average_score' => isset($scores[$key]) ? round($scores[$key], 1) : null,
                'last_seen' => $row->last_seen,
            ];
        })->sortByDesc('interactions')->values()->all();
    }

    /**
     * "AI time" is the summed duration of sessions: stretches of work on one task
     * with no gap longer than the idle window.
     *
     * It is NOT wall-clock time between the first and last prompt of the day, which
     * would count lunch, and it is not the sum of response times, which would count
     * only the seconds the model was typing. The dashboard states this definition
     * on the page, because a metric people cannot explain is a metric they will
     * argue with.
     */
    private function secondsPerTask(): array
    {
        return AiSession::query()
            ->where('started_at', '>=', $this->since())
            ->whereNotNull('ended_at')
            ->select([
                'task_id',
                DB::raw('sum(extract(epoch from (ended_at - started_at))) as seconds'),
            ])
            ->groupBy('task_id')
            ->pluck('seconds', 'task_id')
            ->mapWithKeys(fn ($seconds, $task) => [(string) $task => (int) $seconds])
            ->all();
    }

    private function averageScorePerTask(): array
    {
        return AiInteraction::query()
            ->join('quality_scores', 'quality_scores.ai_interaction_id', '=', 'ai_interactions.id')
            ->where('ai_interactions.occurred_at', '>=', $this->since())
            ->select(['ai_interactions.task_id', DB::raw('avg(quality_scores.score) as average')])
            ->groupBy('ai_interactions.task_id')
            ->pluck('average', 'task_id')
            ->mapWithKeys(fn ($average, $task) => [(string) $task => (float) $average])
            ->all();
    }

    /**
     * Usage per project, where a project is the repository the work happened in.
     *
     * This is the unit the dashboard is built on. Tickets are not: a ticket key in
     * a branch name is a convention this team does not follow, and a rule that
     * only reports for teams who already write "ABC-123" in their branches
     * reports nothing at all. A checkout is something every interaction has.
     */
    public function perProject(): array
    {
        $interactions = AiInteraction::query()
            ->where('occurred_at', '>=', $this->since())
            ->select([
                'repo',
                DB::raw('count(*) as interaction_count'),
                DB::raw('count(*) filter (where automated = false) as human_prompts'),
                DB::raw('count(*) filter (where automated = true) as automated_followups'),
                DB::raw('sum(prompt_tokens + response_tokens) as tokens'),
                DB::raw('count(distinct branch) as branches'),
                DB::raw('max(occurred_at) as last_seen'),
            ])
            ->groupBy('repo')
            ->get();

        $time = $this->secondsPerProject();
        $scores = $this->averageScorePerProject();

        return $interactions->map(function ($row) use ($time, $scores) {
            $key = (string) ($row->repo ?? '');

            return [
                'repo' => $row->repo,
                // The last path segment is what a person calls the project; the
                // full path is kept because two checkouts can share a name.
                'name' => $row->repo ? basename($row->repo) : null,
                'unknown' => blank($row->repo),
                'interactions' => (int) $row->interaction_count,
                'human_prompts' => (int) $row->human_prompts,
                'automated_followups' => (int) $row->automated_followups,
                'branches' => (int) $row->branches,
                'tokens' => (int) $row->tokens,
                'ai_seconds' => (int) ($time[$key] ?? 0),
                'average_score' => isset($scores[$key]) ? round($scores[$key], 1) : null,
                'last_seen' => $row->last_seen,
            ];
        })->sortByDesc('interactions')->values()->all();
    }

    private function secondsPerProject(): array
    {
        return AiSession::query()
            ->where('started_at', '>=', $this->since())
            ->whereNotNull('ended_at')
            ->select(['repo', DB::raw('sum(extract(epoch from (ended_at - started_at))) as seconds')])
            ->groupBy('repo')
            ->pluck('seconds', 'repo')
            ->mapWithKeys(fn ($seconds, $repo) => [(string) $repo => (int) $seconds])
            ->all();
    }

    private function averageScorePerProject(): array
    {
        return AiInteraction::query()
            ->join('quality_scores', 'quality_scores.ai_interaction_id', '=', 'ai_interactions.id')
            ->where('ai_interactions.occurred_at', '>=', $this->since())
            ->select(['ai_interactions.repo', DB::raw('avg(quality_scores.score) as average')])
            ->groupBy('ai_interactions.repo')
            ->pluck('average', 'repo')
            ->mapWithKeys(fn ($average, $repo) => [(string) $repo => (float) $average])
            ->all();
    }

    /** One project's interactions, newest first. */
    public function interactionsForProject(?string $repo, int $perPage = 20): LengthAwarePaginator
    {
        $query = AiInteraction::query()
            ->with('score:id,ai_interaction_id,score')
            ->where('occurred_at', '>=', $this->since())
            ->latest('occurred_at');

        blank($repo) ? $query->whereNull('repo') : $query->where('repo', $repo);

        return $query->paginate($perPage)->through(fn ($i) => [
            'id' => $i->id,
            'tool' => $i->tool,
            'model' => $i->model,
            'branch' => $i->branch,
            'automated' => (bool) $i->automated,
            'prompt_chars' => $i->prompt_chars,
            'answer_chars' => $i->answer_chars,
            'score' => $i->score?->score,
            'occurred_at' => $i->occurred_at,
        ]);
    }

    /** Usage per person, for the same period. */
    public function perPerson(): array
    {
        $rows = AiInteraction::query()
            ->leftJoin('users', 'users.id', '=', 'ai_interactions.user_id')
            ->leftJoin('quality_scores', 'quality_scores.ai_interaction_id', '=', 'ai_interactions.id')
            ->where('ai_interactions.occurred_at', '>=', $this->since())
            ->select([
                'ai_interactions.user_id',
                DB::raw('max(users.name) as name'),
                DB::raw('count(distinct ai_interactions.id) as interactions'),
                DB::raw('count(distinct ai_interactions.task_id) as tasks'),
                DB::raw('avg(quality_scores.score) as average_score'),
            ])
            ->groupBy('ai_interactions.user_id')
            ->get();

        $time = AiSession::query()
            ->where('started_at', '>=', $this->since())
            ->whereNotNull('ended_at')
            ->select(['user_id', DB::raw('sum(extract(epoch from (ended_at - started_at))) as seconds')])
            ->groupBy('user_id')
            ->pluck('seconds', 'user_id');

        return $rows->map(fn ($row) => [
            'user_id' => $row->user_id,
            'name' => $row->name ?? 'Unassigned device',
            'interactions' => (int) $row->interactions,
            'tasks' => (int) $row->tasks,
            'ai_seconds' => (int) ($time[$row->user_id] ?? 0),
            'average_score' => $row->average_score ? round((float) $row->average_score, 1) : null,
        ])->sortByDesc('interactions')->values()->all();
    }

    /**
     * The weakest scoring dimensions across the tenant, with their reasons. This
     * is the coaching view: what everyone could do better, rather than who is
     * worst.
     */
    public function weakestDimensions(): array
    {
        $scores = \App\Models\QualityScore::query()
            ->whereHas('interaction', fn ($q) => $q->where('occurred_at', '>=', $this->since()))
            ->get(['dimensions']);

        $totals = [];

        foreach ($scores as $score) {
            foreach ($score->dimensions ?? [] as $name => $dimension) {
                $totals[$name]['sum'] = ($totals[$name]['sum'] ?? 0) + $dimension['score'];
                $totals[$name]['count'] = ($totals[$name]['count'] ?? 0) + 1;
                $totals[$name]['reasons'][$dimension['reason']] =
                    ($totals[$name]['reasons'][$dimension['reason']] ?? 0) + 1;
            }
        }

        $out = [];

        foreach ($totals as $name => $totalsForName) {
            arsort($totalsForName['reasons']);

            $out[] = [
                'dimension' => $name,
                'average' => round($totalsForName['sum'] / $totalsForName['count'], 1),
                'sample' => $totalsForName['count'],
                'common_reason' => array_key_first($totalsForName['reasons']),
            ];
        }

        usort($out, fn ($a, $b) => $a['average'] <=> $b['average']);

        return $out;
    }

    public function totals(): array
    {
        $interactions = AiInteraction::where('occurred_at', '>=', $this->since());

        return [
            'days' => $this->days,
            'interactions' => (clone $interactions)->count(),
            'human_prompts' => (clone $interactions)->where('automated', false)->count(),
            // Work outside any checkout: a browser, or a tool run from a
            // directory that is not a repository. It has no project to belong to.
            'untagged' => (clone $interactions)->whereNull('repo')->count(),
            'tools' => (clone $interactions)->distinct()->count('tool'),
        ];
    }

    /**
     * One task's interactions, newest first — the drill-down behind the per-task
     * rows. The literal 'untagged' addresses the bucket with no branch ticket;
     * real task IDs never look like that, so there is no collision.
     *
     * Paginated, because the untagged bucket grows without bound and a 100-row
     * dump is how a page starts timing out.
     */
    public function interactionsForTask(string $task, int $perPage = 20): LengthAwarePaginator
    {
        $query = AiInteraction::query()
            ->with('score:id,ai_interaction_id,score')
            ->where('occurred_at', '>=', $this->since())
            ->latest('occurred_at');

        if ($task === 'untagged') {
            $query->whereNull('task_id');
        } else {
            $query->where('task_id', $task);
        }

        return $query->paginate($perPage)->through(fn ($i) => [
            'id' => $i->id,
            'tool' => $i->tool,
            'model' => $i->model,
            'task_id' => $i->task_id,
            'automated' => (bool) $i->automated,
            'prompt_chars' => $i->prompt_chars,
            'score' => $i->score?->score,
            'occurred_at' => $i->occurred_at,
        ]);
    }

    /**
     * Work grouped the way it actually happened: one row per session.
     *
     * A session is a stretch of interactions on one task from one device with no
     * gap longer than the idle window, which is how the ingestion groups them.
     * A flat list of interactions buries the shape of the work — one message to
     * an agent produces a dozen rows — so the dashboard leads with sessions and
     * lets a reader open one.
     */
    public function sessions(int $limit = 25): array
    {
        return AiSession::query()
            ->where('started_at', '>=', $this->since())
            ->withCount([
                'interactions as human_prompts' => fn ($q) => $q->where('automated', false),
            ])
            ->withAvg(
                ['interactions as avg_score' => fn ($q) => $q->join(
                    'quality_scores', 'quality_scores.ai_interaction_id', '=', 'ai_interactions.id'
                )],
                'quality_scores.score'
            )
            ->with('user:id,name')
            ->latest('started_at')
            ->limit($limit)
            ->get()
            ->map(fn (AiSession $s) => [
                'id' => $s->id,
                'tool' => $s->tool,
                'task_id' => $s->task_id,
                'branch' => $s->branch,
                'repo' => $s->repo,
                'project' => $s->repo ? basename($s->repo) : null,
                'person' => $s->user?->name,
                'interactions' => $s->interaction_count,
                'human_prompts' => $s->human_prompts,
                'avg_score' => $s->avg_score === null ? null : round((float) $s->avg_score, 1),
                'started_at' => $s->started_at,
                'ended_at' => $s->ended_at,
                // The model owns this definition, so the session list and the
                // per-task "AI time" can never drift apart.
                'seconds' => $s->durationSeconds(),
            ])->all();
    }

    /** One session's interactions, oldest first: a conversation reads forwards. */
    public function interactionsForSession(AiSession $session): array
    {
        return $session->interactions()
            ->with('score:id,ai_interaction_id,score')
            ->orderBy('occurred_at')
            ->get()
            ->map(fn (AiInteraction $i) => [
                'id' => $i->id,
                'tool' => $i->tool,
                'model' => $i->model,
                'automated' => (bool) $i->automated,
                'prompt_chars' => $i->prompt_chars,
                'answer_chars' => $i->answer_chars,
                'prompt_tokens' => $i->prompt_tokens,
                'response_tokens' => $i->response_tokens,
                'score' => $i->score?->score,
                'occurred_at' => $i->occurred_at,
            ])->all();
    }

    /** The newest interactions across tasks, for the dashboard's recent list. */
    public function recent(int $limit = 10): array
    {
        return AiInteraction::query()
            ->with('score:id,ai_interaction_id,score')
            ->where('occurred_at', '>=', $this->since())
            ->latest('occurred_at')
            ->limit($limit)
            ->get()
            ->map(fn ($i) => [
                'id' => $i->id,
                'tool' => $i->tool,
                'model' => $i->model,
                'project' => $i->repo ? basename($i->repo) : null,
                'branch' => $i->branch,
                'automated' => (bool) $i->automated,
                'score' => $i->score?->score,
                'occurred_at' => $i->occurred_at,
            ])->all();
    }
}
