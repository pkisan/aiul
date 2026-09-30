<?php

namespace Pm\Tests;

use App\Models\ConsentRecord;
use App\Models\Tenant;
use App\Models\User;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Pm\Http\ProjectController;
use Pm\Models\Project;
use Pm\Models\Task;
use Pm\Models\WorkPeriod;
use Tests\TestCase;

class PmCoreTest extends TestCase
{
    use RefreshDatabase;

    private function user(string $role, string $tenant = 'acme'): User
    {
        app(TenantContext::class)->set(null);
        $t = Tenant::firstOrCreate(['slug' => $tenant], ['name' => ucfirst($tenant)]);
        $user = User::create([
            'tenant_id' => $t->id, 'name' => ucfirst($role), 'email' => $role.'-'.uniqid().'@example.com',
            'password' => 'password', 'role' => $role,
        ]);
        ConsentRecord::withoutGlobalScope('tenant')->create([
            'tenant_id' => $t->id, 'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);

        return $user;
    }

    /** A project with one task, created inside the user's tenant. */
    private function project(User $user, array $task = []): Task
    {
        return app(TenantContext::class)->runAs($user->tenant_id, function () use ($task) {
            $project = Project::create(['name' => 'Shop', 'key' => 'SHOP']);

            return $project->tasks()->create($task + ['number' => 1, 'title' => 'Checkout page', 'status' => 'todo']);
        });
    }

    public function test_only_managers_create_projects_and_sprints(): void
    {
        $member = $this->user(User::ROLE_MEMBER);
        $this->actingAs($member)->post(route('pm.projects.store'), ['name' => 'Shop', 'key' => 'SHOP'])->assertForbidden();

        $manager = $this->user(User::ROLE_MANAGER);
        $this->actingAs($manager)->post(route('pm.projects.store'), [
            'name' => 'Shop', 'key' => 'SHOP',
            'remotes' => "git@github.com:acme/shop.git\nhttps://user:tok@GitHub.com/acme/api.git",
        ])->assertRedirect();

        $project = Project::sole();
        $this->assertSame(['github.com/acme/api', 'github.com/acme/shop'], $project->remotes()->orderBy('remote')->pluck('remote')->all());

        $this->actingAs($member)->post(route('pm.sprints.store', $project), ['name' => 'S1', 'start_date' => '2026-10-01', 'end_date' => '2026-10-14'])->assertForbidden();
        $this->actingAs($manager)->post(route('pm.sprints.store', $project), ['name' => 'S1', 'start_date' => '2026-10-01', 'end_date' => '2026-09-01'])->assertSessionHasErrors('end_date');

        // Same key again in this tenant: refused. A remote used by another project: refused.
        $this->actingAs($manager)->post(route('pm.projects.store'), ['name' => 'Again', 'key' => 'SHOP'])->assertSessionHasErrors('key');
        $this->actingAs($manager)->post(route('pm.projects.store'), ['name' => 'Api', 'key' => 'API', 'remotes' => 'github.com/acme/api'])->assertSessionHasErrors('remotes');
    }

    public function test_remote_normalising_matches_the_agent(): void
    {
        $cases = [
            'https://github.com/acme/shop.git' => 'github.com/acme/shop',
            'git@github.com:acme/shop.git' => 'github.com/acme/shop',
            'ssh://git@GitHub.com:22/acme/shop' => 'github.com/acme/shop',
            'https://u:p@ss@gitlab.com/g/sub/repo/' => 'gitlab.com/g/sub/repo',
            'github.com/acme/shop' => 'github.com/acme/shop',
            '/Users/priya/code/shop' => null,
            '../shop' => null,
            'C:\\repos\\shop' => null,
            'file:///srv/shop.git' => null,
        ];
        foreach ($cases as $in => $out) {
            $this->assertSame($out, ProjectController::normaliseRemote($in), $in);
        }
    }

    public function test_another_tenant_cannot_see_or_touch_tasks(): void
    {
        $task = $this->project($this->user(User::ROLE_MEMBER));
        $stranger = $this->user(User::ROLE_MANAGER, 'globex');

        $this->actingAs($stranger)->get(route('pm.tasks.show', $task))->assertNotFound();
        $this->actingAs($stranger)->get(route('pm.board', $task->project_id))->assertNotFound();
        $this->actingAs($stranger)->patch(route('pm.tasks.update', $task), ['status' => 'done'])->assertNotFound();
        $this->actingAs($stranger)->post(route('pm.tasks.start', $task))->assertNotFound();
    }

    public function test_tasks_get_numbers_and_assignees_stay_in_the_tenant(): void
    {
        $me = $this->user(User::ROLE_MEMBER);
        $task = $this->project($me);
        $outsider = $this->user(User::ROLE_MEMBER, 'globex');

        $this->actingAs($me)->post(route('pm.tasks.store', $task->project_id), ['title' => 'Cart'])->assertSessionHasNoErrors();
        $this->assertSame([1, 2], Task::orderBy('number')->pluck('number')->all());

        $this->actingAs($me)->post(route('pm.tasks.store', $task->project_id), ['title' => 'X', 'assignee_id' => $outsider->id])->assertSessionHasErrors('assignee_id');
        $this->actingAs($me)->patch(route('pm.tasks.update', $task), ['assignee_id' => $outsider->id])->assertSessionHasErrors('assignee_id');

        $this->actingAs($me)->get(route('pm.board', $task->project_id))->assertOk()
            ->assertInertia(fn ($page) => $page->component('Pm/Board')->has('tasks', 2)->where('tasks.0.key', 'SHOP-1'));
    }

    public function test_status_changes_are_recorded_with_times(): void
    {
        $me = $this->user(User::ROLE_MEMBER);
        $task = $this->project($me);

        $this->actingAs($me)->patch(route('pm.tasks.update', $task), ['status' => 'in_progress']);
        $started = $task->fresh()->started_at;
        $this->assertNotNull($started);

        $this->travel(2)->days();
        $this->actingAs($me)->patch(route('pm.tasks.update', $task), ['status' => 'done']);
        $this->assertNotNull($task->fresh()->completed_at);
        $this->assertEquals($started, $task->fresh()->started_at, 'started_at is the FIRST move to in progress');

        $this->actingAs($me)->patch(route('pm.tasks.update', $task), ['status' => 'in_progress']);
        $this->assertNull($task->fresh()->completed_at, 'reopened: no longer complete');

        $this->assertSame(
            [['todo', 'in_progress'], ['in_progress', 'done'], ['done', 'in_progress']],
            $task->events()->orderBy('id')->get()->map(fn ($e) => [$e->from, $e->to])->all(),
        );
    }

    public function test_start_working_switches_the_active_task(): void
    {
        $me = $this->user(User::ROLE_MEMBER);
        $first = $this->project($me);
        $second = app(TenantContext::class)->runAs($me->tenant_id, fn () => $first->project->tasks()->create(['number' => 2, 'title' => 'Cart', 'status' => 'backlog']));

        $this->actingAs($me)->post(route('pm.tasks.start', $first))->assertRedirect();
        $this->actingAs($me)->post(route('pm.tasks.start', $second))->assertRedirect();

        $this->assertSame(1, WorkPeriod::whereNull('ended_at')->count(), 'one active task per person');
        $this->assertSame($second->id, WorkPeriod::whereNull('ended_at')->value('task_id'));
        $this->assertSame(['in_progress', $me->id], [$second->fresh()->status, $second->fresh()->assignee_id]);

        $this->actingAs($me)->get(route('pm.tasks.show', $second))
            ->assertInertia(fn ($page) => $page->where('pmActiveTask.key', 'SHOP-2'));

        // Finishing the task ends the work period.
        $this->actingAs($me)->patch(route('pm.tasks.update', $second), ['status' => 'done']);
        $this->assertSame(0, WorkPeriod::whereNull('ended_at')->count());

        $this->actingAs($me)->post(route('pm.tasks.start', $first));
        $this->actingAs($me)->post(route('pm.work.stop'));
        $this->assertSame(0, WorkPeriod::whereNull('ended_at')->count());
    }

    public function test_cannot_start_a_task_assigned_to_someone_else(): void
    {
        $me = $this->user(User::ROLE_MEMBER);
        $other = $this->user(User::ROLE_MEMBER);
        $task = $this->project($me, ['assignee_id' => $other->id]);

        $this->actingAs($me)->post(route('pm.tasks.start', $task))->assertForbidden();
        $this->assertSame(0, WorkPeriod::count());
    }

    public function test_every_screen_renders_for_a_brand_new_tenant(): void
    {
        $manager = $this->user(User::ROLE_MANAGER, 'fresh');

        $this->actingAs($manager)->get(route('pm.home'))->assertRedirect(route('pm.projects.index'));
        foreach (['pm.projects.index', 'pm.pulse', 'pm.tools', 'pm.people.me'] as $name) {
            $this->actingAs($manager)->get(route($name))->assertOk();
        }
        $this->actingAs($manager)->get(route('pm.inbox.index'))->assertOk()->assertJsonPath('total', 0);

        // A project with no tasks, and a task with no AI work.
        $task = $this->project($manager);
        $this->actingAs($manager)->get(route('pm.board', $task->project_id))->assertOk();
        $this->actingAs($manager)->get(route('pm.tasks.show', $task))->assertOk()
            ->assertInertia(fn ($page) => $page->where('trail.sessions', [])->where('trail.totals.prompts', 0));
    }
}
