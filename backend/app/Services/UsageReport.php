<?php

namespace App\Services;

use App\Models\AiInteraction;
use App\Models\AiSession;
use Illuminate\Database\Eloquent\Builder;
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
    /**
     * @param  ?int  $userId  only this person's work
     * @param  ?string  $tool  only this tool
     * @param  ?string  $repo  only this project; '' means "outside any checkout"
     * @param  int  $offsetDays  shift the period back, e.g. $days for "the period
     *                           before this one", which the Overview compares against
     */
    public function __construct(
        private readonly int $days = 30,
        private readonly ?int $userId = null,
        private readonly ?string $tool = null,
        private readonly ?string $repo = null,
        private readonly int $offsetDays = 0,
    ) {}

    private function since(): Carbon
    {
        return now()->subDays($this->days + $this->offsetDays);
    }

    private function until(): Carbon
    {
        return now()->subDays($this->offsetDays);
    }

    /**
     * The period and the page's filters, in one place so no query can forget one.
     * $column is the time column, table-qualified when the query joins: the
     * filters use the same table.
     */
    private function inRange(Builder $query, string $column): Builder
    {
        $table = str_contains($column, '.') ? strstr($column, '.', true).'.' : '';

        return $query->where($column, '>=', $this->since())
            ->when($this->offsetDays > 0, fn ($q) => $q->where($column, '<', $this->until()))
            ->when($this->userId, fn ($q) => $q->where($table.'user_id', $this->userId))
            ->when($this->tool, fn ($q) => $q->where($table.'tool', $this->tool))
            ->when($this->repo !== null, fn ($q) => $this->repo === ''
                ? $q->whereNull($table.'repo')
                : $q->where($table.'repo', $this->repo));
    }

    /** AiInteraction::scopeHumanPrompts as SQL, for counting inside a group. */
    private const HUMAN = "(kind = 'human' or (kind is null and automated = false))";

    /** "AI time" for a query over sessions: see the definition on the page. */
    private const SESSION_SECONDS = 'coalesce(sum(extract(epoch from (ended_at - started_at))), 0)';

    /** The one-line summary at the top of the page. */
    public function summary(): array
    {
        $interactions = AiInteraction::query()
            ->tap(fn ($q) => $this->inRange($q, 'occurred_at'))
            ->selectRaw('count(*) as interactions, count(distinct user_id) as people, count(distinct repo) as projects')
            ->first();

        $sessions = AiSession::query()
            ->tap(fn ($q) => $this->inRange($q, 'started_at'))
            ->whereNotNull('ended_at')
            ->selectRaw('count(*) as sessions, '.self::SESSION_SECONDS.' as seconds')
            ->first();

        return [
            'days' => $this->days,
            'prompts' => AiInteraction::query()->humanPrompts()->tap(fn ($q) => $this->inRange($q, 'occurred_at'))->count(),
            'interactions' => (int) $interactions->interactions,
            'people' => (int) $interactions->people,
            'projects' => (int) $interactions->projects,
            'sessions' => (int) $sessions->sessions,
            'ai_seconds' => (int) $sessions->seconds,
        ];
    }

    /**
     * What people typed, newest first: the page's main list. Agent steps and the
     * tool's own calls are left out — they are on the session page, under the
     * prompt that caused them.
     */
    public function prompts(int $perPage = 25): LengthAwarePaginator
    {
        return AiInteraction::query()
            ->humanPrompts()
            ->tap(fn ($q) => $this->inRange($q, 'occurred_at'))
            ->with('user:id,name')
            ->latest('occurred_at')
            ->latest('id')
            ->paginate($perPage)
            ->withQueryString()
            ->onEachSide(1)
            ->through(fn (AiInteraction $i) => [
                'id' => $i->id,
                'session_id' => $i->ai_session_id,
                'user_id' => $i->user_id,
                'person' => $i->user?->name,
                'account' => $i->account,
                'tool' => $i->tool,
                'model' => $i->model,
                'project' => $i->repo ? basename($i->repo) : null,
                'branch' => $i->branch,
                'prompt_chars' => $i->prompt_chars,
                'answer_chars' => $i->answer_chars,
                'occurred_at' => $i->occurred_at,
                // Filled in by the controller, and only for someone allowed
                // to read prompt text.
                'prompt_object' => $i->prompt_object,
            ]);
    }

    /**
     * Work grouped the way it happened: one row per session, by LAST activity —
     * a session still under way started hours ago, and sorting by start buried
     * it under every session opened since.
     */
    public function sessions(int $perPage = 25): LengthAwarePaginator
    {
        return AiSession::query()
            ->tap(fn ($q) => $this->inRange($q, 'started_at'))
            ->withCount(['interactions as human_prompts' => fn ($q) => $q->humanPrompts()])
            ->with('user:id,name')
            // The AI account the tool named, if any; the person is the fallback.
            ->addSelect(['account' => AiInteraction::select('account')
                ->whereColumn('ai_session_id', 'ai_sessions.id')
                ->whereNotNull('account')
                ->limit(1)])
            ->latest('ended_at')
            ->latest('id')
            ->paginate($perPage)
            ->withQueryString()
            ->onEachSide(1)
            ->through(fn (AiSession $s) => [
                'id' => $s->id,
                'tool' => $s->tool,
                'branch' => $s->branch,
                'repo' => $s->repo,
                'project' => $s->repo ? basename($s->repo) : null,
                'user_id' => $s->user_id,
                'person' => $s->user?->name,
                'account' => $s->account,
                'interactions' => $s->interaction_count,
                'human_prompts' => $s->human_prompts,
                'started_at' => $s->started_at,
                'ended_at' => $s->ended_at,
                'seconds' => $s->durationSeconds(),
            ]);
    }

    /** Per person: prompts, AI time, last seen. Unassigned devices are one row. */
    public function perPerson(): array
    {
        $rows = AiInteraction::query()
            ->leftJoin('users', 'users.id', '=', 'ai_interactions.user_id')
            ->tap(fn ($q) => $this->inRange($q, 'ai_interactions.occurred_at'))
            ->groupBy('ai_interactions.user_id')
            ->select([
                'ai_interactions.user_id',
                DB::raw('max(users.name) as name'),
                DB::raw('count(*) filter (where '.self::HUMAN.') as prompts'),
                DB::raw('count(*) as interactions'),
                DB::raw('max(ai_interactions.occurred_at) as last_seen'),
                // Most-frequent values (Postgres mode(); nulls are ignored).
                DB::raw('mode() within group (order by ai_interactions.tool) as main_tool'),
                DB::raw('mode() within group (order by ai_interactions.repo) as main_repo'),
                DB::raw('count(distinct ai_interactions.occurred_at::date) as active_days'),
                DB::raw("count(*) filter (where ai_interactions.kind = 'agent') as agent_steps"),
            ])
            ->get();

        $time = AiSession::query()
            ->tap(fn ($q) => $this->inRange($q, 'started_at'))
            ->whereNotNull('ended_at')
            ->groupBy('user_id')
            ->selectRaw('user_id, '.self::SESSION_SECONDS.' as seconds')
            ->pluck('seconds', 'user_id');

        return $rows->map(fn ($row) => [
            'user_id' => $row->user_id,
            'name' => $row->name ?? 'Unassigned device',
            'prompts' => (int) $row->prompts,
            'interactions' => (int) $row->interactions,
            'ai_seconds' => (int) ($time[$row->user_id] ?? 0),
            'main_tool' => $row->main_tool,
            'main_project' => $row->main_repo ? basename($row->main_repo) : null,
            'active_days' => (int) $row->active_days,
            'agent_steps' => (int) $row->agent_steps,
            'last_seen' => $row->last_seen,
        ])->sortByDesc('prompts')->values()->all();
    }

    /**
     * How the team works with AI, as four ratios: how much of it is project
     * work, how much the AI does on its own per request, how long a working
     * stretch runs, and how many days a person uses it.
     */
    public function patterns(): array
    {
        $row = AiInteraction::query()
            ->tap(fn ($q) => $this->inRange($q, 'occurred_at'))
            ->selectRaw('count(*) filter (where '.self::HUMAN.') as prompts')
            ->selectRaw('count(*) filter (where '.self::HUMAN.' and repo is not null) as project_prompts')
            ->selectRaw("count(*) filter (where kind = 'agent') as agent_steps")
            ->selectRaw('count(distinct ai_session_id) filter (where '.self::HUMAN.') as sessions')
            ->selectRaw('count(distinct (user_id, occurred_at::date)) as person_days')
            ->selectRaw('count(distinct user_id) as people')
            ->first();

        $ratio = fn ($a, $b) => $b > 0 ? round($a / $b, 1) : null;

        return [
            'project_share' => $row->prompts > 0 ? (int) round($row->project_prompts / $row->prompts * 100) : null,
            'steps_per_prompt' => $ratio($row->agent_steps, $row->prompts),
            'prompts_per_session' => $ratio($row->prompts, $row->sessions),
            'days_per_person' => $ratio($row->person_days, $row->people),
        ];
    }

    /** Prompts per tool, for the "which tools" bars. */
    public function perTool(): array
    {
        return AiInteraction::query()
            ->humanPrompts()
            ->tap(fn ($q) => $this->inRange($q, 'occurred_at'))
            ->groupBy('tool')
            ->selectRaw('tool, count(*) as prompts')
            ->orderByDesc('prompts')
            ->get()
            ->map(fn ($row) => ['tool' => $row->tool, 'prompts' => (int) $row->prompts])
            ->all();
    }

    /**
     * Per project, where a project is the repository the work happened in.
     * Work outside any checkout (a browser chat) is its own row, never hidden.
     */
    public function perProject(): array
    {
        $rows = AiInteraction::query()
            ->tap(fn ($q) => $this->inRange($q, 'occurred_at'))
            ->groupBy('repo')
            ->select([
                'repo',
                DB::raw('count(*) filter (where '.self::HUMAN.') as prompts'),
                DB::raw('count(*) as interactions'),
                DB::raw('count(distinct user_id) as people'),
                DB::raw('max(occurred_at) as last_seen'),
            ])
            ->get();

        $time = AiSession::query()
            ->tap(fn ($q) => $this->inRange($q, 'started_at'))
            ->whereNotNull('ended_at')
            ->groupBy('repo')
            ->selectRaw('repo, '.self::SESSION_SECONDS.' as seconds')
            ->pluck('seconds', 'repo');

        return $rows
            ->map(fn ($row) => [
                'repo' => $row->repo,
                // The last path segment is what a person calls the project; the
                // full path is kept because two checkouts can share a name.
                'name' => $row->repo ? basename($row->repo) : null,
                'prompts' => (int) $row->prompts,
                'interactions' => (int) $row->interactions,
                'people' => (int) $row->people,
                // pluck() keys a null repo as '', the same bucket.
                'ai_seconds' => (int) ($time[$row->repo ?? ''] ?? 0),
                'last_seen' => $row->last_seen,
            ])
            ->sortByDesc('interactions')->values()->all();
    }

    /**
     * Prompts over time, one series per value of $column ('tool' or 'repo'),
     * for the Overview's chart and trend lines. The bucket grows with the
     * period so a chart always has a readable number of bars.
     *
     * ponytail: buckets are cut in the app timezone (UTC); a per-tenant
     * timezone if a team's "day" must start at their midnight.
     *
     * @return array{unit: string, buckets: list<string>, series: array<string, list<int>>}
     */
    public function perBucket(string $column): array
    {
        abort_unless(in_array($column, ['tool', 'repo'], true), 500);

        $unit = match (true) {
            $this->days <= 1 => 'hour',
            $this->days <= 30 => 'day',
            $this->days <= 90 => 'week',
            default => 'month',
        };

        // Every bucket in the period, empty ones included: a gap is information.
        $buckets = [];
        $start = $unit === 'week' ? $this->since()->startOfWeek(Carbon::MONDAY) : $this->since()->startOf($unit);
        for ($at = $start; $at <= $this->until(); $at = $at->copy()->add(1, $unit)) {
            $buckets[] = $at->format('Y-m-d H:i:s');
        }
        $index = array_flip($buckets);

        $rows = AiInteraction::query()
            ->humanPrompts()
            ->tap(fn ($q) => $this->inRange($q, 'occurred_at'))
            ->groupBy('bucket', $column)
            ->selectRaw("date_trunc('{$unit}', occurred_at) as bucket, {$column} as key, count(*) as prompts")
            ->get();

        $series = [];
        foreach ($rows as $row) {
            $at = Carbon::parse($row->bucket)->format('Y-m-d H:i:s');
            if (! isset($index[$at])) {
                continue;
            }
            $key = $row->key ?? '';
            $series[$key] ??= array_fill(0, count($buckets), 0);
            $series[$key][$index[$at]] += (int) $row->prompts;
        }

        return ['unit' => $unit, 'buckets' => $buckets, 'series' => $series];
    }

    /** One project's interactions, newest first. */
    public function interactionsForProject(?string $repo, int $perPage = 25): LengthAwarePaginator
    {
        $query = AiInteraction::query()
            ->with('user:id,name')
            ->tap(fn ($q) => $this->inRange($q, 'occurred_at'))
            ->latest('occurred_at')
            ->latest('id');

        blank($repo) ? $query->whereNull('repo') : $query->where('repo', $repo);

        return $query->paginate($perPage)->withQueryString()->onEachSide(1)->through(fn ($i) => [
            'id' => $i->id,
            'session_id' => $i->ai_session_id,
            'person' => $i->user?->name,
            'tool' => $i->tool,
            'model' => $i->model,
            'branch' => $i->branch,
            'kind' => $i->kind ?? ($i->automated ? 'agent' : 'human'),
            'prompt_chars' => $i->prompt_chars,
            'answer_chars' => $i->answer_chars,
            'occurred_at' => $i->occurred_at,
        ]);
    }

    /**
     * One session's interactions, oldest first and grouped into turns.
     *
     * A turn is one thing a person asked for: the message they typed, every
     * request the agent made working on it, and the answer it came back with.
     *
     * The grouping is computed here rather than stored: it is a reading of the
     * sequence, and a stored turn id would be wrong the moment the rule improves.
     */
    public function interactionsForSession(AiSession $session, bool $withPreviews = false): array
    {
        $interactions = $session->interactions()
            ->orderBy('occurred_at')
            ->orderBy('id')
            ->get();

        $rows = $interactions->map(fn (AiInteraction $i) => [
            'id' => $i->id,
            'tool' => $i->tool,
            'model' => $i->model,
            'account' => $i->account,
            // Rows captured before kinds existed only have the old boolean.
            'kind' => $i->kind ?? ($i->automated ? 'agent' : 'human'),
            'legacy_kind' => $i->kind === null,
            'prompt_chars' => $i->prompt_chars,
            'answer_chars' => $i->answer_chars,
            'prompt_tokens' => $i->prompt_tokens,
            'response_tokens' => $i->response_tokens,
            'occurred_at' => $i->occurred_at,
        ])->all();

        // Rows stored before ingestion recognised agent-written "prompts".
        $bodies = app(BodyStore::class);
        $prompts = [];
        foreach ($interactions as $index => $i) {
            $prompts[$index] = PromptText::clean($bodies->get($i->prompt_object));
            if ($rows[$index]['kind'] === 'human' && PromptText::isToolGenerated($prompts[$index])) {
                $rows[$index]['kind'] = 'utility';
            }
        }

        $rows = $this->intoTurns($rows);

        if (! $withPreviews) {
            return array_map(fn ($row) => $row + ['prompt_preview' => null, 'answer_preview' => null], $rows);
        }

        // The page reads as a chat: what the person typed and the answer they
        // read are shown whole; the agent's own steps in between only by their
        // opening lines. The full text of any step is on its interaction page.
        foreach ($rows as $index => $row) {
            $i = $interactions[$index];
            $prompt = $prompts[$index];
            $answer = $bodies->get($i->answer_object);

            $rows[$index]['prompt_preview'] = $row['kind'] === 'human' ? (blank($prompt) ? null : $prompt) : self::preview($prompt);
            $rows[$index]['answer_preview'] = $row['final_answer'] ? (blank($answer) ? null : $answer) : self::preview($answer);
        }

        return $rows;
    }

    /** The opening of a body, on one line, for a list that has to stay readable. */
    public static function preview(?string $text, int $limit = 300): ?string
    {
        if (blank($text)) {
            return null;
        }

        $flat = trim(preg_replace('/\s+/u', ' ', $text));

        return mb_strlen($flat) > $limit ? mb_substr($flat, 0, $limit).'…' : $flat;
    }

    /**
     * Number each row with the turn it belongs to, and mark the row that carries
     * the answer the person actually read.
     *
     * The final answer of a turn is its LAST row with any answer text: an agent
     * ends a turn by writing a reply rather than calling another tool. Utility
     * calls belong to no turn — they are the tool talking to itself — so they
     * keep the current turn number but can never be its answer.
     */
    private function intoTurns(array $rows): array
    {
        $turn = 0;
        $lastAnswerIndex = [];

        foreach ($rows as $index => $row) {
            if ($row['kind'] === 'human') {
                $turn++;
            }

            $rows[$index]['turn'] = $turn;
            $rows[$index]['final_answer'] = false;

            if ($turn > 0 && $row['kind'] !== 'utility' && ($row['answer_chars'] ?? 0) > 0) {
                $lastAnswerIndex[$turn] = $index;
            }
        }

        foreach ($lastAnswerIndex as $index) {
            $rows[$index]['final_answer'] = true;
        }

        return $rows;
    }
}
