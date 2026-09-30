<?php

namespace Pm\Database\Seeders;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\Device;
use App\Models\Tenant;
use App\Models\User;
use App\Services\BodyStore;
use App\Support\TenantContext;
use Illuminate\Database\Seeder;
use Illuminate\Support\Carbon;
use Illuminate\Support\Str;
use Pm\Linking\Linker;
use Pm\Models\Project;
use Pm\Models\Sprint;
use Pm\Models\Task;
use Pm\Models\TaskEvent;
use Pm\Models\WorkPeriod;
use RuntimeException;

/**
 * Made-up PM data from the founder's sketch: tenant "Aayatti", five people with
 * their AI tools, project AAY, one sprint, tasks on every status, and the AI
 * sessions behind them. The signature example is Parit's AAY-1 on Mon 14 Sep
 * 2026, 10:34 AM to 2:15 PM, prompts 1.1 to 1.4 with Claude.
 *
 *   php artisan db:seed --class='Pm\Database\Seeders\PmDemoSeeder'
 *
 * Rerunning deletes the tenant (everything cascades) and rebuilds it. Refuses
 * production. Times are written in IST and stored in UTC.
 *
 * Links are not written here: the linker runs at the end, exactly as in real
 * use. Expected: AAY-1 explicit, AAY-2 by branch key, AAY-3/4/5/10 by time
 * window (inbox suggestions), Mann's two personal chats unlinked (inbox).
 */
class PmDemoSeeder extends Seeder
{
    private const TZ = 'Asia/Kolkata';

    private const REMOTE = 'github.com/aayatti/pm';

    // name => [tool, host, model]; the tool names are the ones the logger records
    private const PEOPLE = [
        'Parit' => ['claude-code', 'api.anthropic.com', 'claude-sonnet-5-5'],
        'Saurabh' => ['antigravity', 'cloudcode-pa.googleapis.com', 'gemini-3-pro'],
        'Pardeep' => ['claude-code', 'api.anthropic.com', 'claude-sonnet-5-5'],
        'Aakash' => ['claude-web', 'claude.ai', 'claude-opus-5-5'],
        'Mann' => ['chatgpt-web', 'chatgpt.com', 'gpt-5'],
    ];

    // number => [title, status, assignee, started (IST), completed (IST), branch, prompts]
    private const TASKS = [
        1 => ['Kanban board with five columns', 'done', 'Parit', '2026-09-14 10:30', '2026-09-16 17:00', 'feature/board', []],
        2 => ['Sign in with the company Google account', 'done', 'Pardeep', '2026-09-15 11:00', '2026-09-22 16:00', 'feature/AAY-2-google-sign-in', [
            'Add Google sign-in to Laravel with Socialite, company domain only.',
            'Socialite returns an invalid state exception after the redirect. Why?',
            'Write a test that a gmail.com account is refused.',
        ]],
        3 => ['Link AI sessions to tasks by branch name', 'in_progress', 'Saurabh', '2026-09-21 10:00', null, 'main', [
            'Regex for a task key like AAY-42 inside a git branch name.',
            'The branch feature/aay-42-fix has a lower-case key. Should it match?',
            'Why does my query return every session twice?',
            'Rewrite this linker loop so it does not run one query per session.',
            'Still getting duplicates. Here is the SQL again.',
        ]],
        4 => ['Task AI Trail timeline', 'in_progress', 'Parit', '2026-09-24 10:15', null, 'trail-timeline', [
            'Vertical timeline in Vue: each row a prompt and its answer, collapsed to one line.',
            'Number the rows 1.1, 1.2 per session. Where should that number be computed?',
            'Make the expanded answer keep line breaks and code blocks.',
        ]],
        5 => ['Cost per task from token counts', 'in_review', 'Aakash', '2026-09-22 12:00', null, null, [
            'Price table for Claude, GPT and Gemini models per million tokens, as a PHP config array.',
            'Cost when a model is missing from the table: zero, or show unknown?',
            'Round cost to cents or keep four decimals for sums?',
        ]],
        6 => ['Sprint burndown export to CSV', 'todo', 'Mann', null, null, null, []],
        7 => ['Unlinked inbox drawer', 'todo', 'Pardeep', null, null, null, []],
        8 => ['Dark mode for the board', 'backlog', null, null, null, null, []],
        9 => ['Weekly email summary for managers', 'backlog', null, null, null, null, []],
        10 => ['Fix duplicate task on double click', 'done', 'Mann', '2026-09-17 10:00', '2026-09-18 15:30', null, [
            'A button submits a form twice when double clicked. Plain JS fix?',
            'Also guard it on the server: unique index or a lock?',
        ]],
    ];

    // Parit's AAY-1 trail: prompt, answer, time (IST).
    private const TRAIL = [
        ['Plan a Kanban board in Vue with five columns: backlog, todo, in progress, in review, done.', "Five columns from one ordered status list, one component per card.\n\n1. `STATUSES` constant shared by PHP and Vue.\n2. `Board.vue` loops the columns.\n3. `Card.vue` shows key, title and assignee.", '10:34'],
        ['Write the Laravel migration and model for tasks with a status column.', "```php\n\$table->string('status', 16)->default('backlog');\n```\nKeep the allowed values in a constant on the model and validate against it.", '11:48'],
        ['Drag and drop between columns: which library, or plain HTML5 drag events?', 'Plain HTML5 drag events are enough for five columns: dragstart stores the task id, drop sends a PATCH. Add a library only if you need touch support.', '13:02'],
        ['Write a feature test that moving a card updates the task status.', "```php\n\$this->patch(route('tasks.move', \$task), ['status' => 'done'])->assertRedirect();\n\$this->assertSame('done', \$task->fresh()->status);\n```", '14:15'],
    ];

    // Web chats that are not task work (for the "Not task work" button).
    private const PERSONAL = ['Draft a polite email asking for Friday off.', 'Explain the difference between TCP and UDP simply.'];

    private BodyStore $bodies;

    private bool $withBodies = true;

    /** @var array<string, array{User, Device}> */
    private array $people = [];

    public function run(): void
    {
        if (app()->environment('production')) {
            throw new RuntimeException('PmDemoSeeder writes made-up data; it never runs in production.');
        }

        mt_srand(14); // the same demo every time
        app(TenantContext::class)->set(null);
        $this->bodies = app(BodyStore::class);

        Tenant::where('slug', 'aayatti')->delete(); // cascades to everything below
        $tenant = Tenant::create(['name' => 'Aayatti', 'slug' => 'aayatti', 'retention_days' => 90]);
        // Everything created from here on gets this tenant_id (BelongsToTenant).
        app(TenantContext::class)->set($tenant->id);

        $password = Str::password(16, symbols: false);
        $this->person('Aayatti Manager', 'manager@aayatti.test', User::ROLE_MANAGER, $password);
        foreach (array_keys(self::PEOPLE) as $name) {
            $user = $this->person($name, strtolower($name).'@aayatti.test', User::ROLE_MEMBER, Str::password(20));
            $device = Device::create(['user_id' => $user->id, 'hostname' => strtolower($name).'-mbp', 'platform' => 'darwin', 'token_hash' => Device::issueToken()[1]]);
            $this->people[$name] = [$user, $device];
        }

        $project = Project::create(['name' => 'Aayatti PM', 'key' => 'AAY']);
        $project->remotes()->create(['remote' => self::REMOTE]);
        $sprint = Sprint::create(['project_id' => $project->id, 'name' => 'Sprint 1', 'start_date' => '2026-09-14', 'end_date' => '2026-10-02']);

        foreach (self::TASKS as $number => [$title, $status, $assignee, $started, $completed, $branch, $prompts]) {
            $task = $this->task($project, $sprint, $number, $title, $status, $assignee, $started, $completed);
            if ($assignee && $started) {
                $this->workOn($task, $assignee, $this->ist($started), $completed ? $this->ist($completed) : $this->now(), $branch, $prompts);
            }
        }

        $this->trail(Task::where('number', 1)->firstOrFail());

        foreach (self::PERSONAL as $i => $prompt) {
            $this->session('Mann', $this->ist('2026-09-2'.(3 + $i).' 18:10'), null, [$prompt]);
        }

        app(TenantContext::class)->set(null);
        $linked = app(Linker::class)->relink(startedSince: $this->ist('2026-09-01 00:00'));
        $this->command->info("Linker looked at {$linked} sessions.");
        $this->command->info('Aayatti ready: 5 people, project AAY, '.count(self::TASKS).' tasks'.($this->withBodies ? '' : ' (no prompt text: object storage unreachable)').'.');
        $this->command->warn('Sign in as manager@aayatti.test with password (shown once):');
        $this->command->line('    '.$password);
    }

    private function person(string $name, string $email, string $role, string $password): User
    {
        $user = User::create(['tenant_id' => app(TenantContext::class)->id(), 'name' => $name, 'email' => $email, 'role' => $role, 'password' => $password]);
        $user->forceFill(['email_verified_at' => now()])->save();
        ConsentRecord::create([
            'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);

        return $user;
    }

    /** A task plus the status events that got it to where it is. */
    private function task(Project $project, Sprint $sprint, int $number, string $title, string $status, ?string $assignee, ?string $started, ?string $completed): Task
    {
        $task = Task::create([
            'project_id' => $project->id, 'sprint_id' => $status === 'backlog' ? null : $sprint->id,
            'number' => $number, 'title' => $title, 'status' => $status,
            'assignee_id' => $assignee ? $this->people[$assignee][0]->id : null,
            'started_at' => $started ? $this->ist($started) : null,
            'completed_at' => $completed ? $this->ist($completed) : null,
        ]);

        $by = $assignee ? $this->people[$assignee][0]->id : null;
        $steps = [];
        if ($started) {
            $steps[] = ['in_progress', $this->ist($started)];
        }
        if ($status === 'in_review') {
            $steps[] = ['in_review', $this->ist($started)->addDays(4)];
        }
        if ($completed) {
            $steps[] = ['done', $this->ist($completed)];
        }
        $from = 'todo';
        foreach ($steps as [$to, $at]) {
            TaskEvent::create(['task_id' => $task->id, 'user_id' => $by, 'field' => 'status', 'from' => $from, 'to' => $to, 'occurred_at' => $at]);
            $from = $to;
        }

        return $task;
    }

    /**
     * One AI session per working day from $from to $to, prompts drawn from the
     * task's list. Saurabh's AAY-3 gets many prompts a day and no status change:
     * the "stuck" example.
     */
    private function workOn(Task $task, string $name, Carbon $from, Carbon $to, ?string $branch, array $prompts): void
    {
        if (! $prompts) {
            return; // AAY-1 has its own hand-written trail
        }
        $stuck = $task->number === 3;
        for ($day = $from->copy()->startOfDay(); $day->lte($to); $day->addDay()) {
            if ($day->isWeekend()) {
                continue;
            }
            $start = $day->copy()->setTimezone(self::TZ)->setTime(mt_rand(10, 16), mt_rand(0, 59))->utc();
            if ($start->gt($this->now())) {
                break;
            }
            $count = $stuck ? mt_rand(6, 9) : mt_rand(2, 4);
            $picked = array_map(fn () => $prompts[array_rand($prompts)], range(1, $count));
            $this->session($name, $start, $branch, $picked);
        }
    }

    /** Parit's AAY-1: work period, one session 10:34 to 14:15, prompts 1.1-1.4 (the linker makes it explicit). */
    private function trail(Task $task): void
    {
        [$user] = $this->people['Parit'];
        WorkPeriod::create(['user_id' => $user->id, 'task_id' => $task->id, 'started_at' => $this->ist('2026-09-14 10:30'), 'ended_at' => $this->ist('2026-09-14 14:30')]);

        $turns = array_map(fn ($t) => [$t[0], $t[1], $this->ist('2026-09-14 '.$t[2])], self::TRAIL);
        $this->session('Parit', $turns[0][2], 'feature/board', $turns);

    }

    /**
     * One AI session. $turns is a list of prompt strings, or of [prompt, answer,
     * time] for hand-written ones. Code tools also take agent steps every ~8
     * minutes between prompts, as Claude Code does; that is also what keeps a long
     * session under the logger's 30-minute idle split.
     */
    private function session(string $name, Carbon $at, ?string $branch, array $turns): AiSession
    {
        [$user, $device] = $this->people[$name];
        [$tool, $host, $model] = self::PEOPLE[$name];
        $inRepo = in_array($tool, ['claude-code', 'antigravity'], true);

        $session = AiSession::create([
            'device_id' => $device->id, 'user_id' => $user->id, 'tool' => $tool,
            'repo' => $inRepo ? '/Users/'.strtolower($name).'/code/pm' : null,
            'remote' => $inRepo ? self::REMOTE : null,
            'branch' => $inRepo ? $branch : null,
            'started_at' => $at, 'ended_at' => $at, 'interaction_count' => 0,
        ]);

        $time = $at->copy();
        $n = 0;
        foreach ($turns as $turn) {
            [$prompt, $answer, $when] = is_array($turn) ? $turn : [$turn, "Here is one way to do it:\n\n1. Start with the smallest change.\n2. Add a test that fails first.\n3. Then refactor.", null];
            if ($when) {
                // Fill the gap up to this prompt with agent steps, as the tool would.
                while ($inRepo && $n > 0 && $time->copy()->addMinutes(8)->lt($when)) {
                    $this->interaction($session, $host, $model, null, null, $time->addMinutes(8));
                    $n++;
                }
                $time = $when->copy();
            }
            $this->interaction($session, $host, $model, $prompt, $answer, $time);
            $n++;
            if ($inRepo && ! $when) {
                for ($s = mt_rand(1, 3); $s > 0; $s--) {
                    $this->interaction($session, $host, $model, null, null, $time->addSeconds(mt_rand(20, 90)));
                    $n++;
                }
            }
            if (! $when) {
                $time->addMinutes(mt_rand(3, 12));
            }
        }

        $session->update(['ended_at' => $time, 'interaction_count' => $n]);

        return $session;
    }

    /** $prompt null = an automated agent step, which has no human-written prompt. */
    private function interaction(AiSession $session, string $host, string $model, ?string $prompt, ?string $answer, Carbon $at): void
    {
        $eventId = 'pmdemo-'.Str::uuid();
        AiInteraction::create([
            'device_id' => $session->device_id, 'user_id' => $session->user_id, 'ai_session_id' => $session->id,
            'event_id' => $eventId, 'host' => $host, 'path' => '/v1/messages',
            'tool' => $session->tool, 'model' => $model,
            'kind' => $prompt ? 'human' : 'agent', 'automated' => $prompt === null,
            'repo' => $session->repo, 'branch' => $session->branch, 'remote' => $session->remote,
            'prompt_chars' => strlen($prompt ?? 'tool result'), 'answer_chars' => strlen($answer ?? 'tool call'),
            'prompt_tokens' => mt_rand(400, 9000), 'response_tokens' => mt_rand(80, 1500),
            'request_bytes' => mt_rand(2000, 40000), 'response_bytes' => mt_rand(500, 9000),
            'duration_ms' => mt_rand(800, 25000), 'streamed' => true,
            'redaction_rules_version' => 2, 'allowlist_version' => 4,
            'prompt_object' => $prompt ? $this->body($session->tenant_id, $eventId, 'prompt', $prompt) : null,
            'answer_object' => $answer ? $this->body($session->tenant_id, $eventId, 'answer', $answer) : null,
            'occurred_at' => $at,
        ]);
    }

    /** Prompt text; the numbers still work if object storage is down. */
    private function body(int $tenantId, string $eventId, string $part, string $text): ?string
    {
        if (! $this->withBodies) {
            return null;
        }
        try {
            return $this->bodies->put($tenantId, $eventId, $part, $text);
        } catch (\Throwable) {
            $this->withBodies = false;

            return null;
        }
    }

    private function ist(string $time): Carbon
    {
        return Carbon::parse($time, self::TZ)->utc();
    }

    /** "Now" for the demo: real now, so in-progress work runs up to today. */
    private function now(): Carbon
    {
        return now()->utc();
    }
}
