<?php

namespace App\Http\Controllers;

use App\Models\AiInteraction;
use App\Models\ConsentRecord;
use App\Services\BodyStore;
use App\Services\UsageReport;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Gate;
use Inertia\Inertia;
use Inertia\Response;

class UsageDashboardController extends Controller
{
    public function __construct(private readonly BodyStore $bodies) {}

    /** The manager's view: usage per task and per person, and where prompts are weak. */
    public function index(Request $request): Response
    {
        abort_unless($request->user()->isManager(), 403);

        $report = new UsageReport(days: (int) $request->integer('days', 30) ?: 30);

        return Inertia::render('Usage/Index', [
            'totals' => $report->totals(),
            'perTask' => $report->perTask(),
            'perPerson' => $report->perPerson(),
            'weakest' => $report->weakestDimensions(),
            'aiTimeDefinition' => $this->aiTimeDefinition(),
            'canViewRaw' => $request->user()->canViewRawPrompts(),
            'recent' => $report->recent(),
        ]);
    }

    /** One task's interactions: the click between the per-task rows and a prompt. */
    public function task(Request $request, string $task): Response
    {
        abort_unless($request->user()->isManager(), 403);

        $report = new UsageReport(days: (int) $request->integer('days', 30) ?: 30);

        return Inertia::render('Usage/Task', [
            'task' => $task,
            'untagged' => $task === 'untagged',
            'interactions' => $report->interactionsForTask($task),
        ]);
    }

    /** One interaction: metadata and score for anyone allowed to see it. */
    public function show(Request $request, AiInteraction $interaction): Response
    {
        Gate::authorize('view', $interaction);

        return Inertia::render('Usage/Interaction', [
            'interaction' => $interaction->only([
                'id', 'host', 'path', 'tool', 'model', 'task_id', 'branch', 'repo',
                'prompt_chars', 'answer_chars', 'prompt_tokens', 'response_tokens',
                'duration_ms', 'streamed', 'automated', 'redacted', 'occurred_at',
            ]),
            'score' => $interaction->score?->only(['score', 'rubric_version', 'dimensions', 'reasons']),
            'canViewRaw' => Gate::allows('viewRaw', $interaction),
        ]);
    }

    /**
     * The raw prompt and answer text.
     *
     * Two things happen here that do not happen anywhere else: the policy is
     * checked, and the view is written to the audit log BEFORE the text is
     * returned. Logging first matters — if the log write fails, nobody reads
     * anything.
     */
    public function raw(Request $request, AiInteraction $interaction)
    {
        Gate::authorize('viewRaw', $interaction);

        $reason = $request->string('reason')->trim()->toString();

        ConsentRecord::create([
            'tenant_id' => $interaction->tenant_id,
            'user_id' => $interaction->user_id ?? $request->user()->id,
            'kind' => ConsentRecord::KIND_RAW_VIEW,
            'actor_user_id' => $request->user()->id,
            'ai_interaction_id' => $interaction->id,
            'reason' => $reason ?: 'no reason given',
            'ip' => $request->ip(),
        ]);

        return Inertia::render('Usage/Raw', [
            'interaction' => $interaction->only(['id', 'tool', 'model', 'task_id', 'occurred_at', 'redacted']),
            'prompt' => $this->bodies->get($interaction->prompt_object),
            'answer' => $this->bodies->get($interaction->answer_object),
            // Shown on the page: the person reading should know it was recorded.
            'auditNotice' => 'This view has been recorded in the audit log.',
        ]);
    }

    /** The audit log itself, so "who looked at what" is not a private matter. */
    public function audit(Request $request): Response
    {
        abort_unless($request->user()->isManager(), 403);

        return Inertia::render('Usage/Audit', [
            'views' => ConsentRecord::where('kind', ConsentRecord::KIND_RAW_VIEW)
                ->with(['tenant'])
                ->latest()
                ->limit(200)
                ->get()
                ->map(fn ($record) => [
                    'id' => $record->id,
                    'at' => $record->created_at,
                    'actor' => \App\Models\User::find($record->actor_user_id)?->name ?? 'unknown',
                    'subject' => \App\Models\User::find($record->user_id)?->name ?? 'unassigned',
                    'interaction_id' => $record->ai_interaction_id,
                    'reason' => $record->reason,
                    'ip' => $record->ip,
                ]),
        ]);
    }

    /**
     * "My data": what has been captured about the person asking.
     *
     * Everyone can see this about themselves, whatever their role. Transparency
     * is the part that makes the rest acceptable.
     */
    public function myData(Request $request): Response
    {
        $user = $request->user();

        $interactions = AiInteraction::where('user_id', $user->id)
            ->with('score:id,ai_interaction_id,score')
            ->latest('occurred_at')
            ->limit(100)
            ->get();

        return Inertia::render('Usage/MyData', [
            'summary' => [
                'total' => AiInteraction::where('user_id', $user->id)->count(),
                'first_seen' => AiInteraction::where('user_id', $user->id)->min('occurred_at'),
                'devices' => \App\Models\Device::where('user_id', $user->id)
                    ->get(['id', 'hostname', 'platform', 'last_seen_at']),
            ],
            'interactions' => $interactions->map(fn ($i) => [
                'id' => $i->id,
                'occurred_at' => $i->occurred_at,
                'tool' => $i->tool,
                'model' => $i->model,
                'task_id' => $i->task_id,
                'automated' => $i->automated,
                'prompt_chars' => $i->prompt_chars,
                'redacted' => $i->redacted,
                'score' => $i->score?->score,
            ]),
            'whoLooked' => ConsentRecord::where('user_id', $user->id)
                ->where('kind', ConsentRecord::KIND_RAW_VIEW)
                ->where('actor_user_id', '!=', $user->id)
                ->latest()
                ->limit(50)
                ->get()
                ->map(fn ($record) => [
                    'at' => $record->created_at,
                    'actor' => \App\Models\User::find($record->actor_user_id)?->name ?? 'unknown',
                    'reason' => $record->reason,
                ]),
            'explanation' => [
                'Prompts and answers you send to AI tools on a managed device are captured.',
                'Secrets and obvious personal data are masked before anything is stored.',
                'Traffic to anything that is not an AI provider is never decrypted or logged.',
                'Anyone who opens your actual prompt text appears in the list below.',
            ],
        ]);
    }

    private function aiTimeDefinition(): string
    {
        $idle = config('aiul.session_idle_minutes', 30);

        return "AI time is the total length of working sessions: stretches of interactions "
            ."on one task with no gap longer than {$idle} minutes. It is not wall-clock time "
            ."between your first and last prompt, and it is not the time the model spent "
            .'generating.';
    }
}
