<?php

namespace Pm\Tests;

use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\Device;
use App\Models\Tenant;
use App\Models\User;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Pm\Linking\Linker;
use Pm\Models\Project;
use Pm\Models\Task;
use Pm\Models\TaskAiLink;
use Pm\Models\WorkPeriod;
use Tests\TestCase;

class LinkerTest extends TestCase
{
    use RefreshDatabase;

    private Tenant $tenant;

    private User $me;

    private Project $project;

    protected function setUp(): void
    {
        parent::setUp();
        $this->tenant = Tenant::create(['slug' => 'acme', 'name' => 'Acme']);
        app(TenantContext::class)->set($this->tenant->id);
        $this->me = $this->person(User::ROLE_MEMBER);
        $this->project = Project::create(['name' => 'Shop', 'key' => 'SHOP']);
        $this->project->remotes()->create(['remote' => 'github.com/acme/shop']);
    }

    private function person(string $role, ?Tenant $tenant = null): User
    {
        $tenant ??= $this->tenant;
        $user = User::create(['tenant_id' => $tenant->id, 'name' => ucfirst($role), 'email' => uniqid().'@example.com', 'password' => 'password', 'role' => $role]);
        ConsentRecord::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id, 'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);

        return $user;
    }

    private function task(int $number, array $attrs = [], ?Project $project = null): Task
    {
        return ($project ?? $this->project)->tasks()->create($attrs + ['number' => $number, 'title' => "Task {$number}", 'status' => 'todo']);
    }

    private function aiSession(array $attrs = [], ?User $user = null): AiSession
    {
        $user ??= $this->me;
        $device = Device::withoutGlobalScope('tenant')->create(['tenant_id' => $user->tenant_id, 'user_id' => $user->id, 'hostname' => uniqid(), 'platform' => 'darwin', 'token_hash' => Device::issueToken()[1]]);

        return AiSession::withoutGlobalScope('tenant')->create($attrs + [
            'tenant_id' => $user->tenant_id, 'device_id' => $device->id, 'user_id' => $user->id, 'tool' => 'claude-code',
            'started_at' => now()->subHour(), 'ended_at' => now(), 'interaction_count' => 1,
        ]);
    }

    private function link(AiSession $s): ?array
    {
        $link = app(Linker::class)->link($s);

        return $link ? [$link->task_id, $link->method, $link->confidence] : null;
    }

    public function test_explicit_work_period_wins_over_everything(): void
    {
        $a = $this->task(1, ['assignee_id' => $this->me->id, 'started_at' => now()->subDay()]);
        $b = $this->task(2);
        WorkPeriod::create(['user_id' => $this->me->id, 'task_id' => $b->id, 'started_at' => now()->subHours(2)]);

        $s = $this->aiSession(['branch' => 'feature/SHOP-1-cart']);
        $this->assertSame([$b->id, TaskAiLink::EXPLICIT, 1.0], $this->link($s));

        // A period that ended before the session started does not count.
        WorkPeriod::query()->update(['ended_at' => now()->subMinutes(90)]);
        $this->assertSame([$a->id, TaskAiLink::CONVENTION, 0.9], $this->link($s));
    }

    public function test_task_key_in_branch_any_case(): void
    {
        $t = $this->task(42);
        $this->assertSame([$t->id, TaskAiLink::CONVENTION, 0.9], $this->link($this->aiSession(['branch' => 'shop-42-checkout'])));
        $this->assertNull($this->link($this->aiSession(['branch' => 'shop-420'])), 'SHOP-420 is not SHOP-42');
        $this->assertNull($this->link($this->aiSession(['branch' => 'fix/utf-8'])), 'no project UTF');

        $this->task(43);
        $this->assertNull($this->link($this->aiSession(['branch' => 'SHOP-42-and-SHOP-43'])), 'two keys: a guess');
    }

    public function test_only_task_in_progress_in_the_sessions_project(): void
    {
        $t = $this->task(1, ['assignee_id' => $this->me->id, 'started_at' => now()->subDays(2)]);
        $this->task(2, ['assignee_id' => $this->me->id, 'started_at' => now()->subDays(5), 'completed_at' => now()->subDays(3)]);
        $other = Project::create(['name' => 'Blog', 'key' => 'BLOG']);
        $this->task(1, ['assignee_id' => $this->me->id, 'started_at' => now()->subDays(2)], $other);

        // In the shop repository: only SHOP-1 fits.
        $this->assertSame([$t->id, TaskAiLink::TIME_WINDOW, 0.6], $this->link($this->aiSession(['remote' => 'github.com/acme/shop'])));
        // A web chat (no repository): SHOP-1 and BLOG-1 both fit, so no link.
        $this->assertNull($this->link($this->aiSession()));
    }

    public function test_a_confirmed_link_is_never_changed_and_stale_links_go(): void
    {
        $t = $this->task(1, ['assignee_id' => $this->me->id, 'started_at' => now()->subDay()]);
        $s = $this->aiSession();
        $this->assertSame($t->id, $this->link($s)[0]);

        // The task is moved to someone else: the automatic link no longer holds.
        $t->update(['assignee_id' => null]);
        $this->assertNull($this->link($s));
        $this->assertSame(0, TaskAiLink::count());

        TaskAiLink::create(['ai_session_id' => $s->id, 'task_id' => null, 'method' => TaskAiLink::MANUAL, 'confidence' => 1.0, 'confirmed_by' => $this->me->id, 'confirmed_at' => now()]);
        $t->update(['assignee_id' => $this->me->id]);
        $this->assertNull(app(Linker::class)->link($s)->task_id, 'the person said not task work');
    }

    public function test_start_working_then_the_scheduled_command_links_new_sessions(): void
    {
        $t = $this->task(1);
        $this->actingAs($this->me)->post(route('pm.tasks.start', $t));
        $s = $this->aiSession(['started_at' => now()->addMinute()]);

        $this->artisan('pm:link')->assertSuccessful();
        $this->assertSame(TaskAiLink::EXPLICIT, TaskAiLink::where('ai_session_id', $s->id)->value('method'));
    }

    public function test_inbox_lists_weak_and_missing_links_and_takes_decisions(): void
    {
        $t = $this->task(1, ['assignee_id' => $this->me->id, 'started_at' => now()->subDay()]);
        $weak = $this->aiSession();                        // time window, 0.6
        $none = $this->aiSession(['started_at' => now()->subDays(2)]); // before the task started
        $strong = $this->aiSession(['branch' => 'shop-1']); // convention, 0.9: not in the inbox
        app(Linker::class)->relink();

        $this->actingAs($this->me)->get(route('pm.inbox.index'))->assertOk()
            ->assertJsonPath('total', 2)
            ->assertJsonPath('items.0.id', $weak->id)
            ->assertJsonPath('items.0.suggestion.key', 'SHOP-1')
            ->assertJsonPath('items.1.suggestion', null);

        // Someone else's session: a member may not decide it, a manager may.
        $colleague = $this->person(User::ROLE_MEMBER);
        $this->actingAs($colleague)->postJson(route('pm.inbox.decide', $weak), ['action' => 'confirm'])->assertForbidden();
        $this->actingAs($colleague)->postJson(route('pm.inbox.decide', $none), ['action' => 'confirm'])->assertForbidden();

        $this->actingAs($this->me)->postJson(route('pm.inbox.decide', $weak), ['action' => 'confirm'])->assertNoContent();
        $this->actingAs($this->me)->postJson(route('pm.inbox.decide', $none), ['action' => 'none'])->assertNoContent();

        $this->assertEquals([1.0, $this->me->id], [TaskAiLink::where('ai_session_id', $weak->id)->value('confidence'), TaskAiLink::where('ai_session_id', $weak->id)->value('confirmed_by')]);
        $this->assertNull(TaskAiLink::where('ai_session_id', $none->id)->value('task_id'));
        $this->actingAs($this->me)->get(route('pm.inbox.index'))->assertJsonPath('total', 0);

        $manager = $this->person(User::ROLE_MANAGER);
        $this->actingAs($manager)->postJson(route('pm.inbox.decide', $strong), ['action' => 'assign', 'task_id' => $t->id])->assertNoContent();
        $this->assertSame(TaskAiLink::MANUAL, TaskAiLink::where('ai_session_id', $strong->id)->value('method'));
    }

    public function test_inbox_is_per_tenant_and_team_view_is_for_managers(): void
    {
        $this->aiSession();
        $colleague = $this->person(User::ROLE_MEMBER);
        $this->aiSession([], $colleague);

        $this->actingAs($colleague)->get(route('pm.inbox.index', ['scope' => 'team']))->assertJsonPath('total', 1)->assertJsonPath('team', false);
        $this->actingAs($this->person(User::ROLE_MANAGER))->get(route('pm.inbox.index', ['scope' => 'team']))->assertJsonPath('total', 2);

        $mine = $this->aiSession();
        $stranger = $this->person(User::ROLE_MANAGER, Tenant::create(['slug' => 'globex', 'name' => 'Globex']));
        $this->actingAs($stranger)->postJson(route('pm.inbox.decide', $mine), ['action' => 'none'])->assertNotFound();
        $this->actingAs($stranger)->get(route('pm.inbox.index', ['scope' => 'team']))->assertJsonPath('total', 0);

        $this->actingAs($this->me)->get(route('pm.projects.index'))->assertInertia(fn ($p) => $p->where('pmInboxCount', 2));
    }
}
