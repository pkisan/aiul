<?php

namespace Pm\Http;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\User;
use App\Services\BodyStore;
use App\Services\PromptText;
use App\Services\UsageReport;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Http\Response;
use Illuminate\Validation\Rule;
use Pm\Models\Task;
use Pm\Models\TaskAiLink;

/**
 * The unlinked inbox: AI sessions from the last 30 days with no link, or only a
 * weak one (below 0.8), that nobody has confirmed. The person whose session it
 * is, or a manager, confirms it, picks the task, or marks it "not task work".
 */
class InboxController
{
    private const DAYS = 30;

    private const LIMIT = 20;

    /** Sessions waiting for a decision, for these people. Also feeds the nav badge. */
    public static function waiting(array|int|null $userIds): Builder
    {
        return AiSession::query()
            ->when($userIds !== null, fn ($q) => $q->whereIn('user_id', (array) $userIds))
            ->whereNotNull('user_id')
            ->where('started_at', '>=', now()->subDays(self::DAYS))
            ->whereNotExists(fn ($q) => $q->from('pm_task_ai_links')
                ->whereColumn('pm_task_ai_links.ai_session_id', 'ai_sessions.id')
                ->where(fn ($q) => $q->whereNotNull('confirmed_by')->orWhere('confidence', '>=', 0.8)));
    }

    public function index(Request $request, BodyStore $bodies): JsonResponse
    {
        $me = $request->user();
        $team = $request->query('scope') === 'team' && $me->isManager();
        $query = self::waiting($team ? null : $me->id);

        $sessions = (clone $query)->with('user:id,name')->latest('started_at')->limit(self::LIMIT)->get();
        $links = TaskAiLink::with('task.project')->whereIn('ai_session_id', $sessions->pluck('id'))->get()->keyBy('ai_session_id');
        $firsts = AiInteraction::whereIn('ai_session_id', $sessions->pluck('id'))->where('kind', 'human')
            ->orderBy('occurred_at')->get(['ai_session_id', 'prompt_object'])->unique('ai_session_id')->keyBy('ai_session_id');
        $prompts = AiInteraction::whereIn('ai_session_id', $sessions->pluck('id'))->where('kind', 'human')
            ->groupBy('ai_session_id')->selectRaw('ai_session_id, count(*) n')->pluck('n', 'ai_session_id');

        return response()->json([
            'total' => $query->count(),
            'team' => $team,
            // ponytail: one body fetch per item (20 at most); batch if the store is slow.
            'items' => $sessions->map(fn (AiSession $s) => [
                'id' => $s->id,
                'person' => $s->user?->name,
                'tool' => $s->tool,
                'started_at' => $s->started_at,
                'ended_at' => $s->ended_at,
                'where' => $s->remote ?? $s->repo,
                'branch' => $s->branch,
                'prompts' => (int) ($prompts[$s->id] ?? 0),
                // Managers see prompt text too (owner's decision, 2026-09-30).
                'preview' => UsageReport::preview(PromptText::clean($bodies->get($firsts[$s->id]->prompt_object ?? null)), 160),
                'suggestion' => ($link = $links[$s->id] ?? null) ? [
                    'task_id' => $link->task_id, 'key' => $link->task->key, 'title' => $link->task->title,
                    'method' => $link->method, 'confidence' => $link->confidence,
                ] : null,
            ]),
            // ponytail: every task in the tenant; group or search when there are hundreds.
            'tasks' => Task::with('project:id,key')->orderBy('project_id')->orderByDesc('number')->get()
                ->map(fn (Task $t) => ['id' => $t->id, 'key' => $t->key, 'title' => $t->title, 'status' => $t->status, 'mine' => $t->assignee_id === $me->id]),
        ]);
    }

    /** confirm: keep the suggestion. assign: this task. none: not task work. All become certain (1.0). */
    public function decide(Request $request, AiSession $session): Response
    {
        $me = $request->user();
        abort_unless($session->user_id === $me->id || $me->isManager(), 403);

        $data = $request->validate([
            'action' => ['required', Rule::in(['confirm', 'assign', 'none'])],
            'task_id' => ['required_if:action,assign', 'nullable', Rule::exists('pm_tasks', 'id')->where('tenant_id', $me->tenant_id)],
        ]);

        $link = TaskAiLink::firstOrNew(['ai_session_id' => $session->id]);
        abort_if($data['action'] === 'confirm' && ! $link->task_id, 422, 'Nothing to confirm.');

        $link->fill(match ($data['action']) {
            'confirm' => ['confidence' => 1.0],
            'assign' => ['task_id' => $data['task_id'], 'method' => TaskAiLink::MANUAL, 'confidence' => 1.0],
            'none' => ['task_id' => null, 'method' => TaskAiLink::MANUAL, 'confidence' => 1.0],
        })->fill(['confirmed_by' => $me->id, 'confirmed_at' => now()])->save();

        return response()->noContent();
    }

    /** For the badge: how many of my sessions wait. */
    public static function countFor(User $user): int
    {
        return self::waiting($user->id)->count();
    }
}
