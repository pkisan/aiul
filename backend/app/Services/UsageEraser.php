<?php

namespace App\Services;

use App\Models\AiInteraction;
use App\Models\AiSession;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Support\Collection;
use Illuminate\Support\Str;

/**
 * Deletes captured usage. Shared by `aiul:delete-user-data`, the admin's Delete
 * buttons and `aiul:purge-deleted`.
 *
 * Two kinds of delete:
 *   trash()  soft: rows are hidden everywhere, text kept, restorable until
 *            aiul:purge-deleted removes them after config('aiul.trash_days');
 *   delete() permanent: text and rows gone now (a leaked secret, the CLI).
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

    /**
     * Soft delete: hide these rows as one deletion, restorable as one.
     * Returns the deletion id.
     */
    public function trash(int $tenantId, Collection $ids, ?int $byUserId): string
    {
        $deletionId = (string) Str::uuid();
        $rows = AiInteraction::withoutGlobalScope('tenant')->where('tenant_id', $tenantId)->whereIn('id', $ids);
        $sessions = (clone $rows)->pluck('ai_session_id')->filter()->unique();

        $rows->update(['deleted_at' => now(), 'deleted_by' => $byUserId, 'deletion_id' => $deletionId]);
        $this->tidySessions($tenantId, $sessions);

        return $deletionId;
    }

    /** Bring a deletion back, and the sessions it had emptied. */
    public function restore(int $tenantId, string $deletionId): int
    {
        $rows = $this->deletion($tenantId, $deletionId);
        $sessions = (clone $rows)->pluck('ai_session_id')->filter()->unique();

        $count = $rows->update(['deleted_at' => null, 'deleted_by' => null, 'deletion_id' => null]);
        $this->tidySessions($tenantId, $sessions);

        return $count;
    }

    /** Rows of one deletion (they are all trashed). */
    public function deletion(int $tenantId, string $deletionId): Builder
    {
        return AiInteraction::withoutGlobalScope('tenant')->onlyTrashed()
            ->where('tenant_id', $tenantId)
            ->where('deletion_id', $deletionId);
    }

    /** Permanently delete these rows (trashed or not) and their stored text. */
    public function delete(int $tenantId, Collection $ids): int
    {
        $rows = fn () => AiInteraction::withoutGlobalScope('tenant')->withTrashed()->where('tenant_id', $tenantId);
        $sessions = collect();

        foreach ($ids->chunk(200) as $chunk) {
            $found = $rows()->whereIn('id', $chunk)->get(['id', 'ai_session_id', 'prompt_object', 'answer_object']);
            foreach ($found as $row) {
                $this->bodies->delete($row->prompt_object);
                $this->bodies->delete($row->answer_object);
            }
            $sessions = $sessions->merge($found->pluck('ai_session_id')->filter());
            // Scores go with the row (cascade).
            $rows()->whereIn('id', $chunk)->forceDelete();
        }

        $this->tidySessions($tenantId, $sessions->unique(), permanent: true);

        return $ids->count();
    }

    private function inSession(AiInteraction $row): Builder
    {
        return AiInteraction::withoutGlobalScope('tenant')
            ->where('tenant_id', $row->tenant_id)
            ->where('ai_session_id', $row->ai_session_id);
    }

    /**
     * Sessions whose rows changed: re-measure from the rows still visible. A
     * session with none left is hidden (soft deleted) so a restore can bring it
     * back; once nothing at all is left, not even deleted rows, it goes for good.
     */
    private function tidySessions(int $tenantId, Collection $sessionIds, bool $permanent = false): void
    {
        foreach ($sessionIds as $id) {
            $session = AiSession::withoutGlobalScope('tenant')->withTrashed()->where('tenant_id', $tenantId)->find($id);
            if (! $session) {
                continue;
            }

            $visible = AiInteraction::withoutGlobalScope('tenant')->where('ai_session_id', $id);
            $count = (clone $visible)->count();

            if ($count === 0) {
                $anyLeft = AiInteraction::withoutGlobalScope('tenant')->withTrashed()->where('ai_session_id', $id)->exists();
                $permanent && ! $anyLeft ? $session->forceDelete() : $session->delete();

                continue;
            }

            $session->forceFill([
                'deleted_at' => null,
                'interaction_count' => $count,
                'started_at' => (clone $visible)->min('occurred_at'),
                'ended_at' => (clone $visible)->max('occurred_at'),
            ])->save();
        }
    }
}
