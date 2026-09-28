<?php

namespace App\Http\Controllers;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\User;
use App\Services\BodyStore;
use App\Services\PromptText;
use App\Services\UsageReport;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Pagination\LengthAwarePaginator;
use Illuminate\Support\Facades\Gate;
use Inertia\Inertia;
use Inertia\Response;

class UsageDashboardController extends Controller
{
    public function __construct(private readonly BodyStore $bodies) {}

    /**
     * The manager's page: who is using which AI tool on what, newest first.
     * Every filter is in the URL, so a filtered view can be bookmarked or shared.
     */
    public function index(Request $request): Response
    {
        abort_unless($request->user()->isManager(), 403);

        $days = $request->integer('days', 30);
        $filters = [
            'days' => in_array($days, self::PERIODS, true) ? $days : 30,
            'person' => $request->integer('person') ?: null,
            'tool' => $request->string('tool')->toString() ?: null,
            // A repository path, or "-" for work outside any checkout.
            'project' => $request->string('project')->toString() ?: null,
            'view' => $request->string('view')->toString() === 'sessions' ? 'sessions' : 'prompts',
        ];

        $report = new UsageReport(
            days: $filters['days'],
            userId: $filters['person'],
            tool: $filters['tool'],
            repo: $filters['project'] === '-' ? '' : $filters['project'],
        );
        $canViewRaw = $request->user()->canViewRawPrompts();

        return Inertia::render('Usage/Index', [
            'filters' => $filters,
            'summary' => $report->summary(),
            'list' => $filters['view'] === 'sessions'
                ? $report->sessions()
                : $this->withPreviews($request, $report->prompts(), $canViewRaw),
            'perPerson' => $report->perPerson(),
            'perTool' => $report->perTool(),
            'perProject' => $report->perProject(),
            // The filters' choices ignore the filters, or picking one person
            // would leave only that person to pick.
            'options' => [
                'people' => User::where('tenant_id', $request->user()->tenant_id)->orderBy('name')->get(['id', 'name']),
                'tools' => AiInteraction::query()->whereNotNull('tool')->distinct()->orderBy('tool')->pluck('tool'),
                'projects' => AiInteraction::query()->whereNotNull('repo')
                    ->where('occurred_at', '>=', now()->subDays($filters['days']))
                    ->distinct()->orderBy('repo')->pluck('repo')
                    ->map(fn ($repo) => ['repo' => $repo, 'name' => basename($repo)]),
            ],
            'aiTimeDefinition' => $this->aiTimeDefinition(),
            'canViewRaw' => $canViewRaw,
        ]);
    }

    private const PERIODS = [1, 7, 30, 90, 365];

    /**
     * The opening of each prompt, for someone allowed to read prompt text.
     *
     * A preview IS prompt text, so it is audited like a session view: one
     * record per person whose words were on the page, not one per row.
     */
    private function withPreviews(Request $request, LengthAwarePaginator $page, bool $canViewRaw): LengthAwarePaginator
    {
        $rows = $page->getCollection();

        if ($canViewRaw) {
            foreach ($rows->groupBy(fn ($row) => $row['user_id'] ?? 0) as $userId => $group) {
                ConsentRecord::create([
                    'tenant_id' => $request->user()->tenant_id,
                    'user_id' => $userId ?: $request->user()->id,
                    'kind' => ConsentRecord::KIND_LIST_VIEW,
                    'actor_user_id' => $request->user()->id,
                    'reason' => 'saw '.$group->count().' prompt preview(s) on the usage page',
                    'ip' => $request->ip(),
                ]);
            }
        }

        // ponytail: one body fetch per row (25 per page); batch or cache if the store is slow.
        return $page->setCollection($rows->map(function ($row) use ($canViewRaw) {
            $object = $row['prompt_object'];
            unset($row['prompt_object']);

            return $row + ['preview' => $canViewRaw
                ? UsageReport::preview(PromptText::clean($this->bodies->get($object)), 220)
                : null];
        }));
    }

    /** One session, read forwards: the work as it actually happened. */
    public function session(Request $request, AiSession $session): Response
    {
        abort_unless($request->user()->isManager(), 403);

        $report = new UsageReport(days: (int) $request->integer('days', 30) ?: 30);
        $canViewRaw = $request->user()->canViewRawPrompts();

        // One record for the page, not one per row: an audit log with forty
        // entries for a single visit is an audit log nobody reads.
        if ($canViewRaw) {
            ConsentRecord::create([
                'tenant_id' => $session->tenant_id,
                'user_id' => $session->user_id ?? $request->user()->id,
                'kind' => ConsentRecord::KIND_SESSION_VIEW,
                'actor_user_id' => $request->user()->id,
                'reason' => 'opened session #'.$session->id,
                'ip' => $request->ip(),
            ]);
        }

        return Inertia::render('Usage/Session', [
            'session' => [
                'id' => $session->id,
                'tool' => $session->tool,
                'project' => $session->repo ? basename($session->repo) : null,
                'branch' => $session->branch,
                'repo' => $session->repo,
                'started_at' => $session->started_at,
                'ended_at' => $session->ended_at,
                'seconds' => $session->durationSeconds(),
                'interactions' => $session->interaction_count,
                // Who did the work: the AI accounts the tool named, and the person
                // the device is linked to (the fallback when the tool named none).
                'accounts' => $session->interactions()->whereNotNull('account')->distinct()->pluck('account'),
                'person' => $session->user?->name,
            ],
            'interactions' => $report->interactionsForSession($session, withPreviews: $canViewRaw),
            'canViewRaw' => $canViewRaw,
            // The sidebar: the same person's other sessions, latest activity
            // first. Metadata only — no prompt text is fetched for it.
            'sidebar' => (new UsageReport(days: 30, userId: $session->user_id))->sessions(40)->items(),
        ]);
    }

    /**
     * One project's interactions.
     *
     * The repository is a path, so it travels as a query parameter rather than a
     * path segment — encoding "/Users/x/Herd/plrb-lms" into a URL segment is a
     * fight with no prize.
     */
    public function project(Request $request): Response
    {
        abort_unless($request->user()->isManager(), 403);

        $report = new UsageReport(days: (int) $request->integer('days', 30) ?: 30);
        $repo = $request->string('repo')->toString();

        return Inertia::render('Usage/Project', [
            'repo' => $repo ?: null,
            'name' => $repo ? basename($repo) : null,
            'interactions' => $report->interactionsForProject($repo ?: null),
            'canViewRaw' => $request->user()->canViewRawPrompts(),
        ]);
    }

    /**
     * One interaction: metadata for anyone allowed to see it, and the
     * prompt and answer text on the same page for anyone allowed to read it.
     *
     * Reading the text is still the privileged part: the policy is checked and the
     * view is written to the audit log BEFORE the text is fetched. If the log
     * write fails, nobody reads anything.
     */
    public function show(Request $request, AiInteraction $interaction): Response
    {
        Gate::authorize('view', $interaction);

        $props = [
            'interaction' => $interaction->only([
                'id', 'host', 'path', 'tool', 'account', 'model', 'kind', 'task_id', 'branch', 'repo',
                'prompt_chars', 'answer_chars', 'prompt_tokens', 'response_tokens',
                'duration_ms', 'streamed', 'automated', 'redacted', 'occurred_at', 'ai_session_id',
            ]),
            'canViewRaw' => Gate::allows('viewRaw', $interaction),
            'person' => $interaction->user_id ? User::find($interaction->user_id)?->name : null,
        ];

        if ($props['canViewRaw']) {
            ConsentRecord::create([
                'tenant_id' => $interaction->tenant_id,
                'user_id' => $interaction->user_id ?? $request->user()->id,
                'kind' => ConsentRecord::KIND_RAW_VIEW,
                'actor_user_id' => $request->user()->id,
                'ai_interaction_id' => $interaction->id,
                'reason' => $request->string('reason')->trim()->toString() ?: 'no reason given',
                'ip' => $request->ip(),
            ]);

            // Cleaned on the way out too: rows stored before PromptText existed
            // still carry the tool's wrappers.
            $prompt = PromptText::clean($this->bodies->get($interaction->prompt_object));
            $answer = $this->bodies->get($interaction->answer_object);

            $props += [
                'prompt' => $prompt,
                'answer' => $answer,
                // A missing body has two honest meanings: retention deleted it
                // (chars were recorded, the key is gone) or nothing was captured.
                'promptState' => $this->bodyState($prompt, $interaction->prompt_chars),
                'answerState' => $this->bodyState($answer, $interaction->answer_chars),
            ];
        }

        return Inertia::render('Usage/Interaction', $props);
    }

    /** The old separate text page: the text now lives on the interaction page. */
    public function raw(Request $request, AiInteraction $interaction): RedirectResponse
    {
        return redirect()->route('usage.show', array_filter([
            'interaction' => $interaction->id,
            'reason' => $request->string('reason')->trim()->toString(),
        ]));
    }

    private function bodyState(?string $text, ?int $chars): string
    {
        if ($text !== null && $text !== '') {
            return 'present';
        }

        return ($chars ?? 0) > 0 ? 'purged' : 'empty';
    }

    /** The audit log itself, so "who looked at what" is not a private matter. */
    public function audit(Request $request): Response
    {
        abort_unless($request->user()->isManager(), 403);

        return Inertia::render('Usage/Audit', [
            'views' => ConsentRecord::whereIn('kind', ConsentRecord::READ_KINDS)
                ->with(['actor:id,name', 'subject:id,name'])
                ->latest()
                ->latest('id')
                ->paginate(50)
                ->through(fn ($record) => [
                    'id' => $record->id,
                    'at' => $record->created_at,
                    'kind' => $record->kind,
                    'actor' => $record->actor?->name ?? 'unknown',
                    'subject' => $record->subject?->name ?? 'unassigned',
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
                'project' => $i->repo ? basename($i->repo) : null,
                'automated' => $i->automated,
                'prompt_chars' => $i->prompt_chars,
                'redacted' => $i->redacted,
            ]),
            'whoLooked' => ConsentRecord::where('user_id', $user->id)
                ->whereIn('kind', ConsentRecord::READ_KINDS)
                ->with('actor:id,name')
                ->where('actor_user_id', '!=', $user->id)
                ->latest()
                ->limit(50)
                ->get()
                ->map(fn ($record) => [
                    'at' => $record->created_at,
                    'actor' => $record->actor?->name ?? 'unknown',
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
