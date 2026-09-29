<?php

namespace App\Http\Controllers;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\Device;
use App\Models\User;
use App\Services\BodyStore;
use App\Services\PromptText;
use App\Services\UsageEraser;
use App\Services\UsageReport;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Pagination\LengthAwarePaginator;
use Illuminate\Support\Facades\Gate;
use Illuminate\Support\Facades\Log;
use Inertia\Inertia;
use Inertia\Response;

class UsageDashboardController extends Controller
{
    public function __construct(private readonly BodyStore $bodies) {}

    /**
     * The manager's Overview: the period at one glance. Every number compares
     * with the period before it, and every row links to the Activity list
     * filtered to it.
     */
    public function index(Request $request): Response
    {
        abort_unless($request->user()->isManager(), 403);

        $days = $request->integer('days', 7);
        $days = in_array($days, self::PERIODS, true) ? $days : 7;
        $now = new UsageReport(days: $days);
        $before = new UsageReport(days: $days, offsetDays: $days);
        $canViewRaw = $request->user()->canViewRawPrompts();

        $summary = $now->summary();
        $previous = $before->summary();
        $perProject = $now->perProject();
        $perTool = $now->perTool();
        $team = $this->team($now->perPerson());
        $enrolled = collect($team)->whereNotNull('user_id')->where('enrolled', true)->count();

        return Inertia::render('Usage/Overview', [
            'days' => $days,
            'summary' => $summary,
            'previous' => $previous,
            'enrolled' => $enrolled,
            'attention' => $this->attention($days),
            'activity' => $now->perBucket('tool'),
            'trends' => $now->perBucket('repo')['series'],
            'perTool' => $perTool,
            'perProject' => $perProject,
            'team' => $team,
            'latest' => $this->withPreviews($now->prompts(5), $canViewRaw)->items(),
            'insights' => $this->insights($summary, $previous, $perTool, $before->perTool(), $perProject, $team),
            'aiTimeDefinition' => $this->aiTimeDefinition(),
        ]);
    }

    /**
     * People with their period's numbers, plus everyone who has a device but
     * did nothing this period: on a team view, silence is a row, not a gap.
     */
    private function team(array $perPerson): array
    {
        $rows = collect($perPerson)->keyBy(fn ($row) => $row['user_id'] ?? 'none');
        $withDevice = Device::where('revoked', false)->whereNotNull('user_id')
            ->with('user:id,name')
            ->get()
            ->groupBy('user_id');

        foreach ($withDevice as $userId => $devices) {
            $rows[$userId] = ($rows[$userId] ?? [
                'user_id' => $userId,
                'name' => $devices->first()->user?->name ?? 'Unknown',
                'prompts' => 0, 'interactions' => 0, 'ai_seconds' => 0,
                'main_tool' => null, 'last_seen' => null,
            ]) + ['enrolled' => true];
        }

        return $rows->map(fn ($row) => $row + ['enrolled' => false])
            ->sortByDesc('prompts')->values()->all();
    }

    /** Things a manager should act on. Empty lists mean all clear. */
    private function attention(int $days): array
    {
        // Prompts where redaction masked something: a person pasted a secret
        // or personal data into an AI tool. The mask names the rule, never the value.
        $secrets = AiInteraction::query()
            ->where('occurred_at', '>=', now()->subDays($days))
            ->whereNotNull('redacted')
            ->whereRaw("redacted <> '[]'::jsonb")
            ->with('user:id,name')
            ->latest('occurred_at');

        // ponytail: the agent sends no heartbeat, so "silent" means nothing was
        // captured — the agent is off OR the person used no AI. Add a heartbeat
        // event to tell the two apart.
        $silentAfter = now()->subDays(3);
        $devices = Device::where('revoked', false)->with('user:id,name')->get();

        return [
            'secrets_total' => (clone $secrets)->count(),
            'secrets' => $secrets->limit(5)->get()->map(fn ($i) => [
                'id' => $i->id,
                'person' => $i->user?->name ?? 'Unassigned device',
                'tool' => $i->tool,
                'rules' => $i->redacted,
                'occurred_at' => $i->occurred_at,
            ]),
            'silent' => $devices
                ->filter(fn ($d) => $d->last_seen_at === null || $d->last_seen_at < $silentAfter)
                ->map(fn ($d) => [
                    'id' => $d->id, 'hostname' => $d->hostname,
                    'person' => $d->user?->name, 'last_seen_at' => $d->last_seen_at,
                ])->values(),
            'unassigned' => $devices->whereNull('user_id')
                ->map(fn ($d) => ['id' => $d->id, 'hostname' => $d->hostname, 'last_seen_at' => $d->last_seen_at])
                ->values(),
        ];
    }

    /**
     * Two or three plain sentences a manager would otherwise have to work out
     * from the numbers. Rules, not a model: no prompt data leaves the server.
     */
    private function insights(array $now, array $before, array $tools, array $toolsBefore, array $projects, array $team): array
    {
        $out = [];

        if ($before['prompts'] > 0 && $now['prompts'] !== $before['prompts']) {
            $change = (int) round(($now['prompts'] - $before['prompts']) / $before['prompts'] * 100);
            $out[] = 'Prompts '.($change > 0 ? 'up' : 'down').' '.abs($change).'% on the previous period.';
        }

        $top = collect($projects)->whereNotNull('repo')->sortByDesc('prompts')->first();
        if ($top && $now['prompts'] > 0 && $top['prompts'] > 0) {
            $out[] = "Most work was in {$top['name']}: ".round($top['prompts'] / $now['prompts'] * 100).'% of prompts.';
        }

        $was = collect($toolsBefore)->pluck('prompts', 'tool');
        $growth = collect($tools)->map(fn ($t) => $t + ['growth' => $t['prompts'] - ($was[$t['tool']] ?? 0)])
            ->sortByDesc('growth')->first();
        if ($growth && $growth['growth'] > 0) {
            // The page turns the tool id into its display name.
            $out[] = ['tool' => $growth['tool'], 'growth' => $growth['growth']];
        }

        $idle = collect($team)->where('enrolled', true)->where('prompts', 0)->count();
        if ($idle > 0) {
            $out[] = $idle.' of '.collect($team)->where('enrolled', true)->count().' enrolled people used no AI tool in this period.';
        }

        return array_slice($out, 0, 3);
    }

    /**
     * Every prompt or session, newest first, filterable by person, tool and
     * project. Every filter is in the URL, so a filtered view can be bookmarked
     * or shared.
     */
    public function activity(Request $request): Response
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

        return Inertia::render('Usage/Activity', [
            'filters' => $filters,
            'summary' => $report->summary(),
            'list' => $filters['view'] === 'sessions'
                ? $report->sessions()
                : $this->withPreviews($report->prompts(), $canViewRaw),
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

    /** The opening of each prompt, for someone allowed to read prompt text. */
    private function withPreviews(LengthAwarePaginator $page, bool $canViewRaw): LengthAwarePaginator
    {
        $rows = $page->getCollection();

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
     * Reading the text is the privileged part: the policy is checked before the
     * text is fetched.
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
            'canDelete' => Gate::allows('delete', $interaction),
            'person' => $interaction->user_id ? User::find($interaction->user_id)?->name : null,
        ];

        if ($props['canViewRaw']) {
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

    /**
     * Delete a prompt and everything that answered it (see UsageEraser::turn).
     * Normally to Deleted prompts (restorable for config('aiul.trash_days'));
     * with `permanent`, gone now — for a secret that slipped past redaction.
     * The log line records who deleted what, never the text.
     */
    public function destroy(Request $request, AiInteraction $interaction, UsageEraser $eraser): RedirectResponse
    {
        Gate::authorize('delete', $interaction);

        $permanent = $request->boolean('permanent');
        $ids = $eraser->turnContaining($interaction);
        $session = $interaction->ai_session_id;
        $permanent
            ? $eraser->delete($interaction->tenant_id, $ids)
            : $eraser->trash($interaction->tenant_id, $ids, $request->user()->id);

        Log::info($permanent ? 'Prompt deleted permanently from the dashboard' : 'Prompt moved to Deleted prompts', [
            'by_user_id' => $request->user()->id,
            'owner_user_id' => $interaction->user_id,
            'interaction_ids' => $ids->all(),
        ]);

        return $session && AiSession::find($session)
            ? redirect()->route('usage.session', $session)
            : redirect()->route('usage.index');
    }

    /** The old separate text page: the text now lives on the interaction page. */
    public function raw(AiInteraction $interaction): RedirectResponse
    {
        return redirect()->route('usage.show', $interaction->id);
    }

    private function bodyState(?string $text, ?int $chars): string
    {
        if ($text !== null && $text !== '') {
            return 'present';
        }

        return ($chars ?? 0) > 0 ? 'purged' : 'empty';
    }

    /**
     * "My data": what has been captured about the person asking. Everyone can
     * see this about themselves, whatever their role.
     */
    public function myData(Request $request): Response
    {
        $user = $request->user();

        return Inertia::render('Usage/MyData', [
            'summary' => [
                'total' => AiInteraction::where('user_id', $user->id)->count(),
                'first_seen' => AiInteraction::where('user_id', $user->id)->min('occurred_at'),
                'devices' => Device::where('user_id', $user->id)
                    ->get(['id', 'hostname', 'platform', 'last_seen_at']),
            ],
            'explanation' => [
                'Prompts and answers you send to AI tools on a managed device are captured.',
                'Secrets and obvious personal data are masked before anything is stored.',
                'Traffic to anything that is not an AI provider is never decrypted or logged.',
                'Only admins given explicit permission can read prompt text.',
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
