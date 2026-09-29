<?php

namespace App\Services;

use App\Models\AiInteraction;
use App\Models\AiSession;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Support\Collection;

/**
 * Deletes captured usage. Shared by `aiul:delete-user-data` and the admin's
 * Delete button on a prompt.
 *
 * A "turn" is what the person typed plus everything that answered it: the
 * agent's steps working on it and the answer. Deleting only the typed line
 * would leave the answer to a question nobody asked.
 *
 * Bodies go first, rows second (as in PurgeBodies): an interruption leaves a row
 * pointing at missing text, never text with no row to find it by.
 */
class UsageEraser
{
    public function __construct(private BodyStore $bodies) {}

    /**
     * The turn that starts at this typed prompt: it, and everything after it in
     * the same session up to (not including) the next typed prompt.
     */
    public function turn(AiInteraction $prompt): Collection
    {
        if (! $prompt->ai_session_id) {
            return collect([$prompt->id]);
        }

        $after = fn ($q) => $q->where('occurred_at', '>', $prompt->occurred_at)
            ->orWhere(fn ($q) => $q->where('occurred_at', $prompt->occurred_at)->where('id', '>', $prompt->id));

        $next = $this->inSession($prompt)->humanPrompts()->where($after)
            ->orderBy('occurred_at')->orderBy('id')->first(['id', 'occurred_at']);

        $turn = $this->inSession($prompt)->where(fn ($q) => $q->where('id', $prompt->id)->orWhere($after));
        if ($next) {
            $turn->where(fn ($q) => $q->where('occurred_at', '<', $next->occurred_at)
                ->orWhere(fn ($q) => $q->where('occurred_at', $next->occurred_at)->where('id', '<', $next->id)));
        }

        return $turn->pluck('id');
    }

    /**
     * The turn any row belongs to: an answer or an agent step is deleted with
     * the prompt it answered. A row before any typed prompt stands alone.
     */
    public function turnContaining(AiInteraction $row): Collection
    {
        if (! $row->ai_session_id) {
            return collect([$row->id]);
        }

        $prompt = $this->inSession($row)->humanPrompts()
            ->where(fn ($q) => $q->where('occurred_at', '<', $row->occurred_at)
                ->orWhere(fn ($q) => $q->where('occurred_at', $row->occurred_at)->where('id', '<=', $row->id)))
            ->orderByDesc('occurred_at')->orderByDesc('id')
            ->first();

        return $prompt ? $this->turn($prompt) : collect([$row->id]);
    }

    /** Delete these rows and their stored text; fix up the sessions they were in. */
    public function delete(int $tenantId, Collection $ids): int
    {
        $rows = fn () => AiInteraction::withoutGlobalScope('tenant')->where('tenant_id', $tenantId);
        $sessions = collect();

        foreach ($ids->chunk(200) as $chunk) {
            $found = $rows()->whereIn('id', $chunk)->get(['id', 'ai_session_id', 'prompt_object', 'answer_object']);
            foreach ($found as $row) {
                $this->bodies->delete($row->prompt_object);
                $this->bodies->delete($row->answer_object);
            }
            $sessions = $sessions->merge($found->pluck('ai_session_id')->filter());
            // Scores go with the row (cascade).
            $rows()->whereIn('id', $chunk)->delete();
        }

        $this->tidySessions($tenantId, $sessions->unique());

        return $ids->count();
    }

    private function inSession(AiInteraction $row): Builder
    {
        return AiInteraction::withoutGlobalScope('tenant')
            ->where('tenant_id', $row->tenant_id)
            ->where('ai_session_id', $row->ai_session_id);
    }

    /** Sessions that lost rows: drop the empty ones, re-measure the rest. */
    private function tidySessions(int $tenantId, Collection $sessionIds): void
    {
        foreach ($sessionIds as $id) {
            $left = AiInteraction::withoutGlobalScope('tenant')->where('ai_session_id', $id);
            $count = (clone $left)->count();
            $session = AiSession::withoutGlobalScope('tenant')->where('tenant_id', $tenantId)->find($id);

            if (! $session) {
                continue;
            }
            if ($count === 0) {
                $session->delete();

                continue;
            }
            $session->forceFill([
                'interaction_count' => $count,
                'started_at' => (clone $left)->min('occurred_at'),
                'ended_at' => (clone $left)->max('occurred_at'),
            ])->save();
        }
    }
}
