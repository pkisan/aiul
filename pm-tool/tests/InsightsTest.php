<?php

namespace Pm\Tests;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\Device;
use App\Models\Tenant;
use App\Models\User;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Storage;
use Illuminate\Support\Str;
use Pm\Metrics\Cost;
use Pm\Models\Project;
use Pm\Models\Task;
use Pm\Models\TaskAiLink;
use Tests\TestCase;

class InsightsTest extends TestCase
{
    use RefreshDatabase;

    private Tenant $tenant;

    private User $me;

    private User $manager;

    private Project $project;

    protected function setUp(): void
    {
        parent::setUp();
        Storage::fake('s3');
        $this->tenant = Tenant::create(['slug' => 'acme', 'name' => 'Acme']);
        app(TenantContext::class)->set($this->tenant->id);
        $this->me = $this->person(User::ROLE_MEMBER);
        $this->manager = $this->person(User::ROLE_MANAGER);
        $this->project = Project::create(['name' => 'Shop', 'key' => 'SHOP']);
    }

    private function person(string $role, ?Tenant $tenant = null): User
    {
        $tenant ??= $this->tenant;
        $user = User::create(['tenant_id' => $tenant->id, 'name' => ucfirst($role).rand(), 'email' => uniqid().'@example.com', 'password' => 'password', 'role' => $role]);
        ConsentRecord::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id, 'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);

        return $user;
    }

    /** A session of $kinds interactions (human|agent), $minutes apart, linked to $task when given. */
    private function work(?Task $task, array $kinds, $at = null, float $confidence = 0.9, string $model = 'claude-sonnet-5-5'): AiSession
    {
        $at = ($at ?? now()->subHours(2))->copy();
        $device = Device::create(['user_id' => $this->me->id, 'hostname' => uniqid(), 'platform' => 'darwin', 'token_hash' => Device::issueToken()[1]]);
        $s = AiSession::create(['device_id' => $device->id, 'user_id' => $this->me->id, 'tool' => 'claude-code', 'started_at' => $at, 'ended_at' => $at->copy()->addMinutes(count($kinds)), 'interaction_count' => count($kinds)]);
        foreach ($kinds as $i => $kind) {
            AiInteraction::create([
                'device_id' => $device->id, 'user_id' => $this->me->id, 'ai_session_id' => $s->id,
                'event_id' => Str::uuid(), 'host' => 'api.anthropic.com', 'tool' => 'claude-code', 'model' => $model,
                'kind' => $kind, 'automated' => $kind !== 'human', 'prompt_tokens' => 1_000_000, 'response_tokens' => 100_000,
                'occurred_at' => $at->copy()->addMinutes($i),
            ]);
        }
        if ($task) {
            TaskAiLink::create(['ai_session_id' => $s->id, 'task_id' => $task->id, 'method' => 'convention', 'confidence' => $confidence]);
        }

        return $s;
    }

    private function task(int $n, array $attrs = []): Task
    {
        return $this->project->tasks()->create($attrs + ['number' => $n, 'title' => "Task {$n}", 'status' => 'todo', 'assignee_id' => $this->me->id]);
    }

    public function test_cost_uses_the_longest_price_prefix_and_leaves_unknown_models_unpriced(): void
    {
        $this->assertSame([2.00, 10.00], Cost::price('claude-sonnet-5-5'));
        $this->assertSame([2.00, 10.00], Cost::price('claude-sonnet-5'));
        $this->assertSame([4.00, 20.00], Cost::price('claude-opus-5-5'));
        $this->assertSame([5.00, 25.00], Cost::price('claude-opus-5'));
        $this->assertSame([1.00, 5.00], Cost::price('claude-haiku-4-5-20251001'));
        $this->assertNull(Cost::price('gpt-5-codex'));
        $this->assertNull(Cost::price('grok-4.6'));
        $this->assertNull(Cost::price(null));

        $this->work(null, ['human'], model: 'claude-opus-5-5'); // 1M in x $4 + 0.1M out x $20 = $6
        $this->work(null, ['human'], model: 'gpt-5');           // unpriced
        $this->assertSame(['usd' => 6.0, 'tokens' => 2_200_000, 'priced_tokens' => 1_100_000], Cost::of(AiInteraction::query()));
    }

    public function test_pulse_numbers(): void
    {
        $withAi = $this->task(1, ['status' => 'done', 'started_at' => now()->subDays(2), 'completed_at' => now()->subDay()]);
        $this->task(2, ['status' => 'done', 'started_at' => now()->subDays(2), 'completed_at' => now()->subDay()]);
        $open = $this->task(3, ['status' => 'in_progress', 'started_at' => now()->subDays(2)]);

        $this->work($withAi, ['human', 'agent', 'human']);            // trusted, 2 prompts
        $this->work($open, ['human'], confidence: 0.6);               // weak: not "linked"
        $notWork = $this->work(null, ['human']);
        TaskAiLink::create(['ai_session_id' => $notWork->id, 'task_id' => null, 'method' => 'manual', 'confidence' => 1, 'confirmed_by' => $this->me->id]);
        $this->work(null, ['human']);                                 // unlinked

        $this->actingAs($this->manager)->get(route('pm.pulse', ['period' => '7']))->assertOk()
            ->assertInertia(fn ($p) => $p->component('Pm/Pulse')
                ->where('kpis.assisted.pct', 50)          // 1 of 2 done tasks
                ->where('kpis.prompts.perTask', 2)        // human prompts only
                ->where('kpis.linked.linked', 1)
                ->where('kpis.linked.total', 3)           // 4 sessions minus the "not task work" one
                ->where('kpis.linked.pct', 33)
                ->where('kpis.linked.waiting', 2)
                ->has('people', 2));
    }

    public function test_a_task_with_many_prompts_and_no_movement_needs_attention(): void
    {
        $stuck = $this->task(1, ['status' => 'in_progress', 'started_at' => now()->subDays(5)]);
        $moving = $this->task(2, ['status' => 'in_progress', 'started_at' => now()->subDays(5)]);
        $this->work($stuck, array_fill(0, 10, 'human'), confidence: 0.6); // suggested links count here
        $this->work($moving, array_fill(0, 12, 'human'));
        $moving->events()->create(['field' => 'status', 'from' => 'todo', 'to' => 'in_progress', 'occurred_at' => now()->subHour()]);
        $this->work($this->task(3, ['status' => 'in_progress']), array_fill(0, 9, 'human')); // below the bar

        $this->actingAs($this->manager)->get(route('pm.pulse'))
            ->assertInertia(fn ($p) => $p->has('attention', 1)->where('attention.0.key', 'SHOP-1')->where('attention.0.prompts', 10));
    }

    public function test_trail_numbers_prompts_and_hides_text_from_other_members(): void
    {
        $task = $this->task(1);
        $first = $this->work($task, ['human', 'agent', 'agent', 'human'], now()->subHours(5));
        $this->work($task, ['human'], now()->subHours(1), confidence: 0.6);
        $this->work(null, ['human']); // not this task

        $this->actingAs($this->me)->get(route('pm.tasks.show', $task))
            ->assertInertia(fn ($p) => $p
                ->where('trail.sessions.0.turns.0.n', '1.1')
                ->where('trail.sessions.0.turns.0.steps', 2)
                ->where('trail.sessions.0.turns.1.n', '1.2')
                ->where('trail.sessions.1.turns.0.n', '2.1')
                ->where('trail.sessions.0.canRead', true)
                ->where('trail.totals.prompts', 3)
                ->where('trail.totals.suggested', 1));

        $colleague = $this->person(User::ROLE_MEMBER);
        $this->actingAs($colleague)->get(route('pm.tasks.show', $task))
            ->assertInertia(fn ($p) => $p->where('trail.sessions.0.canRead', false)->where('trail.sessions.0.turns.0.preview', null));

        $turn = AiInteraction::where('ai_session_id', $first->id)->where('kind', 'human')->first();
        $this->actingAs($colleague)->getJson(route('pm.trail.turn', $turn))->assertForbidden();
        $this->actingAs($this->manager)->getJson(route('pm.trail.turn', $turn))->assertOk()->assertJsonStructure(['prompt', 'answer']);

        $unlinked = AiInteraction::whereNotIn('ai_session_id', TaskAiLink::pluck('ai_session_id'))->first();
        $this->actingAs($this->manager)->getJson(route('pm.trail.turn', $unlinked))->assertNotFound();
    }

    public function test_who_sees_which_screen(): void
    {
        $this->actingAs($this->me)->get(route('pm.pulse'))->assertForbidden();
        $this->actingAs($this->me)->get(route('pm.tools'))->assertForbidden();
        $this->actingAs($this->me)->get(route('pm.people.me'))->assertOk()->assertInertia(fn ($p) => $p->component('Pm/Person')->where('isMe', true));
        $this->actingAs($this->me)->get(route('pm.people.show', $this->manager))->assertForbidden();

        $this->actingAs($this->manager)->get(route('pm.people.show', $this->me))->assertOk();
        $this->actingAs($this->manager)->get(route('pm.tools', ['period' => '30']))->assertOk()->assertInertia(fn ($p) => $p->component('Pm/Tools'));

        $stranger = $this->person(User::ROLE_MANAGER, Tenant::create(['slug' => 'globex', 'name' => 'Globex']));
        $this->actingAs($stranger)->get(route('pm.people.show', $this->me))->assertNotFound();
    }
}
