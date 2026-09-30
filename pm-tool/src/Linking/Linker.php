<?php

namespace Pm\Linking;

use App\Models\AiSession;
use Illuminate\Support\Carbon;
use Pm\Models\LinkEvent;
use Pm\Models\ProjectRemote;
use Pm\Models\Task;
use Pm\Models\TaskAiLink;
use Pm\Models\TaskKey;
use Pm\Models\WorkPeriod;

/**
 * Decides which task an AI session was for. The ladder (D18, D19): the first
 * rule that points at exactly ONE task wins; two candidates is a guess, and a
 * guess goes to the inbox instead.
 *
 *   1. explicit     1.0  the person's "Start working" period covers the session start
 *   2. convention   0.9  a task key (AAY-42, any case) is in the branch name
 *   3. time_window  0.6  the only task assigned to the person that was started and
 *                        not finished at session start, within the project of the
 *                        session's git remote when that is known
 *
 * A link a person confirmed in the inbox is never changed. Everything else is
 * recomputed, so a later "Start working" or a new task re-attributes old work.
 * Every change is written to pm_link_events.
 *
 * Queries filter by tenant_id themselves: this also runs from the console,
 * where no tenant is set and the global scope filters nothing.
 */
class Linker
{
    /** Task keys in a branch: "feature/AAY-42-cart", "aay-42". Not "utf-8x", not "AAY-420" for 42. */
    private const KEY = '/(?<![A-Za-z0-9])([A-Za-z]{2,6})-(\d+)(?!\d)/';

    public function link(AiSession $session): ?TaskAiLink
    {
        $existing = TaskAiLink::withoutGlobalScope('tenant')->where('ai_session_id', $session->id)->first();
        if ($existing?->confirmed_by) {
            return $existing;
        }

        [$taskId, $method, $confidence] = $this->decide($session) ?? [null, null, null];

        if (! $taskId) {
            if ($existing) {
                $existing->delete();
                LinkEvent::record($session->tenant_id, $session->id, $existing->task_id, null, 'unlinked');
            }

            return null;
        }

        // Only a real change goes in the audit log; relinking every 5 minutes must not flood it.
        $changed = ! $existing || $existing->task_id !== $taskId || $existing->method !== $method || $existing->confidence !== $confidence;
        $link = TaskAiLink::withoutGlobalScope('tenant')->updateOrCreate(
            ['ai_session_id' => $session->id],
            ['tenant_id' => $session->tenant_id, 'task_id' => $taskId, 'method' => $method, 'confidence' => $confidence],
        );
        if ($changed) {
            LinkEvent::record($session->tenant_id, $session->id, $existing?->task_id, $taskId, $confidence >= 0.8 ? 'linked' : 'suggested', $method, $confidence);
        }

        return $link;
    }

    /** Relink one person's recent sessions, after a PM change that affects them. */
    public function relinkUser(?int $userId, int $days = 30): void
    {
        if (! $userId) {
            return;
        }
        AiSession::withoutGlobalScope('tenant')
            ->where('user_id', $userId)
            ->where('started_at', '>=', now()->subDays($days))
            ->each(fn (AiSession $s) => $this->link($s));
    }

    /** Relink every session touched since $since (new prompts) or started since $startedSince. Returns the count. */
    public function relink(?Carbon $since = null, ?Carbon $startedSince = null): int
    {
        $count = 0;
        AiSession::withoutGlobalScope('tenant')
            ->when($since, fn ($q) => $q->where('updated_at', '>=', $since))
            ->when($startedSince, fn ($q) => $q->where('started_at', '>=', $startedSince))
            ->chunkById(200, function ($sessions) use (&$count) {
                foreach ($sessions as $s) {
                    $this->link($s);
                    $count++;
                }
            });

        return $count;
    }

    /** @return array{int, string, float}|null */
    private function decide(AiSession $s): ?array
    {
        if (! $s->user_id) {
            return null; // a device nobody is linked to: no person, no task
        }

        $explicit = WorkPeriod::withoutGlobalScope('tenant')
            ->where('tenant_id', $s->tenant_id)->where('user_id', $s->user_id)
            ->where('started_at', '<=', $s->started_at)
            ->where(fn ($q) => $q->whereNull('ended_at')->orWhere('ended_at', '>', $s->started_at))
            ->distinct()->pluck('task_id');
        if ($explicit->count() === 1) {
            return [$explicit->first(), TaskAiLink::EXPLICIT, 1.0];
        }

        $byKey = $this->tasksInBranch($s);
        if ($byKey->count() === 1) {
            return [$byKey->first(), TaskAiLink::CONVENTION, 0.9];
        }

        $projectId = $s->remote
            ? ProjectRemote::withoutGlobalScope('tenant')->where('tenant_id', $s->tenant_id)->where('remote', $s->remote)->value('project_id')
            : null;
        $open = Task::withoutGlobalScope('tenant')
            ->where('tenant_id', $s->tenant_id)->where('assignee_id', $s->user_id)
            ->when($projectId, fn ($q) => $q->where('project_id', $projectId))
            ->where('started_at', '<=', $s->started_at)
            ->where(fn ($q) => $q->whereNull('completed_at')->orWhere('completed_at', '>', $s->started_at))
            ->pluck('id');
        if ($open->count() === 1) {
            return [$open->first(), TaskAiLink::TIME_WINDOW, 0.6];
        }

        return null;
    }

    /**
     * Ids of the tenant's tasks whose keys appear in the session's branch name.
     * Keys come from pm_task_keys, old ones included, so "aay-4-cart" still finds
     * the task after it moved to another project.
     */
    private function tasksInBranch(AiSession $s)
    {
        if (! $s->branch || ! preg_match_all(self::KEY, $s->branch, $m, PREG_SET_ORDER)) {
            return collect();
        }

        return TaskKey::withoutGlobalScope('tenant')
            ->where('tenant_id', $s->tenant_id)
            ->whereIn('key', collect($m)->map(fn ($k) => strtoupper($k[1]).'-'.(int) $k[2])->unique())
            ->distinct()
            ->pluck('task_id');
    }
}
