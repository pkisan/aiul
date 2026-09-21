<?php

namespace App\Services;

use App\Models\AiInteraction;
use App\Models\AiSession;
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
            'untagged' => (clone $interactions)->whereNull('task_id')->count(),
            'tools' => (clone $interactions)->distinct()->count('tool'),
        ];
    }

    /**
     * One task's interactions, newest first — the drill-down behind the per-task
     * rows. The literal 'untagged' addresses the bucket with no branch ticket;
     * real task IDs never look like that, so there is no collision.
     */
    public function interactionsForTask(string $task): array
    {
        $query = AiInteraction::query()
            ->with('score:id,ai_interaction_id,score')
            ->where('occurred_at', '>=', $this->since())
            ->latest('occurred_at')
            ->limit(100);

        if ($task === 'untagged') {
            $query->whereNull('task_id');
        } else {
            $query->where('task_id', $task);
        }

        return $query->get()->map(fn ($i) => [
            'id' => $i->id,
            'tool' => $i->tool,
            'model' => $i->model,
            'task_id' => $i->task_id,
            'automated' => (bool) $i->automated,
            'prompt_chars' => $i->prompt_chars,
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
                'task_id' => $i->task_id,
                'automated' => (bool) $i->automated,
                'score' => $i->score?->score,
                'occurred_at' => $i->occurred_at,
            ])->all();
    }
}
