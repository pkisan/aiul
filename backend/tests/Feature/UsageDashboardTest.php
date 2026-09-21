<?php

namespace Tests\Feature;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\Device;
use App\Models\QualityScore;
use App\Models\Tenant;
use App\Models\User;
use App\Services\BodyStore;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Storage;
use Tests\TestCase;

class UsageDashboardTest extends TestCase
{
    use RefreshDatabase;

    private Tenant $tenant;

    private Device $device;

    protected function setUp(): void
    {
        parent::setUp();

        Storage::fake('s3');
        app(TenantContext::class)->set(null);

        $this->tenant = Tenant::create(['name' => 'Acme', 'slug' => 'acme']);

        [, $hash] = Device::issueToken();
        $this->device = Device::withoutGlobalScope('tenant')->create([
            'tenant_id' => $this->tenant->id,
            'hostname' => 'mac',
            'token_hash' => $hash,
        ]);
    }

    private function user(string $role = User::ROLE_MEMBER, bool $raw = false, ?Tenant $tenant = null): User
    {
        return User::create([
            'tenant_id' => ($tenant ?? $this->tenant)->id,
            'name' => ucfirst($role),
            'email' => $role.'-'.uniqid().'@example.com',
            'password' => 'password',
            'role' => $role,
            'can_view_raw_prompts' => $raw,
        ]);
    }

    private function interaction(array $overrides = [], string $prompt = 'Explain this function.'): AiInteraction
    {
        $eventId = bin2hex(random_bytes(12));

        $session = AiSession::withoutGlobalScope('tenant')->create([
            'tenant_id' => $this->tenant->id,
            'device_id' => $this->device->id,
            'tool' => 'claude-code',
            'task_id' => $overrides['task_id'] ?? 'ABC-123',
            'started_at' => now()->subMinutes(20),
            'ended_at' => now()->subMinutes(5),
            'interaction_count' => 1,
        ]);

        return AiInteraction::withoutGlobalScope('tenant')->create(array_merge([
            'tenant_id' => $this->tenant->id,
            'device_id' => $this->device->id,
            'ai_session_id' => $session->id,
            'event_id' => $eventId,
            'host' => 'api.anthropic.com',
            'tool' => 'claude-code',
            'model' => 'claude-opus-5',
            'task_id' => 'ABC-123',
            'prompt_object' => app(BodyStore::class)->put($this->tenant->id, $eventId, 'prompt', $prompt),
            'answer_object' => app(BodyStore::class)->put($this->tenant->id, $eventId, 'answer', 'An answer.'),
            'occurred_at' => now()->subMinutes(10),
        ], $overrides));
    }

    public function test_a_member_cannot_open_the_manager_dashboard(): void
    {
        $this->actingAs($this->user())->get('/usage')->assertForbidden();
    }

    public function test_a_manager_sees_usage_per_task(): void
    {
        $this->interaction(['task_id' => 'ABC-123']);
        $this->interaction(['task_id' => 'ABC-123']);
        $this->interaction(['task_id' => 'XYZ-9']);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->assertOk();

        $perTask = collect($response->viewData('page')['props']['perTask']);

        $this->assertSame('ABC-123', $perTask->first()['task_id']);
        $this->assertSame(2, $perTask->first()['interactions']);
        $this->assertTrue($perTask->contains(fn ($row) => $row['task_id'] === 'XYZ-9'));
    }

    public function test_untagged_work_gets_its_own_row_rather_than_disappearing(): void
    {
        $this->interaction(['task_id' => null]);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->assertOk();
        $perTask = collect($response->viewData('page')['props']['perTask']);

        $untagged = $perTask->firstWhere('untagged', true);

        $this->assertNotNull($untagged, 'untagged work must appear as its own row');
        $this->assertSame(1, $untagged['interactions']);
    }

    public function test_the_dashboard_states_how_ai_time_is_measured(): void
    {
        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->assertOk();

        // A metric nobody can explain is a metric nobody trusts, so the definition
        // travels with the number.
        $this->assertStringContainsString(
            'no gap longer than',
            $response->viewData('page')['props']['aiTimeDefinition']
        );
    }

    public function test_a_manager_cannot_read_prompt_text_without_the_explicit_permission(): void
    {
        $interaction = $this->interaction();

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get("/usage/{$interaction->id}/raw")
            ->assertForbidden();

        $this->assertSame(0, ConsentRecord::withoutGlobalScope('tenant')->count());
    }

    public function test_an_admin_without_the_flag_still_cannot_read_prompt_text(): void
    {
        $interaction = $this->interaction();

        $this->actingAs($this->user(User::ROLE_ADMIN, raw: false))
            ->get("/usage/{$interaction->id}/raw")
            ->assertForbidden();
    }

    public function test_opening_prompt_text_is_written_to_the_audit_log(): void
    {
        $subject = $this->user();
        $interaction = $this->interaction(['user_id' => $subject->id], 'a prompt about pineapples');
        $admin = $this->user(User::ROLE_ADMIN, raw: true);

        $response = $this->actingAs($admin)
            ->get("/usage/{$interaction->id}/raw?reason=investigating+a+leak")
            ->assertOk();

        $this->assertSame('a prompt about pineapples', $response->viewData('page')['props']['prompt']);

        $record = ConsentRecord::withoutGlobalScope('tenant')->first();

        $this->assertNotNull($record, 'every raw view must be recorded');
        $this->assertSame(ConsentRecord::KIND_RAW_VIEW, $record->kind);
        $this->assertSame($admin->id, $record->actor_user_id);
        $this->assertSame($subject->id, $record->user_id);
        $this->assertSame($interaction->id, $record->ai_interaction_id);
        $this->assertSame('investigating a leak', $record->reason);
    }

    public function test_reading_anyones_prompt_needs_no_reason_but_is_still_recorded(): void
    {
        $subject = $this->user();
        $interaction = $this->interaction(['user_id' => $subject->id]);
        $admin = $this->user(User::ROLE_ADMIN, raw: true);

        // A grant-holding admin opens with one click: no reason typed.
        $this->actingAs($admin)->get("/usage/{$interaction->id}/raw")->assertOk();

        $record = ConsentRecord::withoutGlobalScope('tenant')->first();
        $this->assertNotNull($record, 'every raw view must be recorded');
        $this->assertSame('no reason given', $record->reason);
    }

    public function test_anyone_may_read_their_own_prompt_and_it_is_still_recorded(): void
    {
        $member = $this->user();
        $interaction = $this->interaction(['user_id' => $member->id], 'my own words');

        $response = $this->actingAs($member)->get("/usage/{$interaction->id}/raw")->assertOk();

        $this->assertSame('my own words', $response->viewData('page')['props']['prompt']);
        $this->assertSame(1, ConsentRecord::withoutGlobalScope('tenant')->count());
    }

    public function test_nobody_can_reach_another_tenants_interaction(): void
    {
        $other = Tenant::create(['name' => 'Globex', 'slug' => 'globex']);
        $interaction = $this->interaction();

        $outsider = $this->user(User::ROLE_ADMIN, raw: true, tenant: $other);

        // Not 403 but 404: the row is invisible to them, which is what the global
        // scope means.
        $this->actingAs($outsider)->get("/usage/{$interaction->id}")->assertNotFound();
        $this->actingAs($outsider)->get("/usage/{$interaction->id}/raw")->assertNotFound();
    }

    public function test_my_data_shows_what_was_captured_and_who_looked(): void
    {
        $member = $this->user();
        $interaction = $this->interaction(['user_id' => $member->id]);
        $admin = $this->user(User::ROLE_ADMIN, raw: true);

        // Somebody reads their prompt.
        $this->actingAs($admin)->get("/usage/{$interaction->id}/raw?reason=support+request")->assertOk();

        $response = $this->actingAs($member)->get('/my-data')->assertOk();
        $props = $response->viewData('page')['props'];

        $this->assertSame(1, $props['summary']['total']);
        $this->assertCount(1, $props['interactions']);

        // The point of the page: the person can see who read their words, and why.
        $this->assertCount(1, $props['whoLooked']);
        $this->assertSame($admin->name, $props['whoLooked'][0]['actor']);
        $this->assertSame('support request', $props['whoLooked'][0]['reason']);
    }

    public function test_my_data_is_open_to_every_role(): void
    {
        $this->actingAs($this->user())->get('/my-data')->assertOk();
        $this->actingAs($this->user(User::ROLE_MANAGER))->get('/my-data')->assertOk();
    }

    public function test_a_task_page_lists_only_that_tasks_interactions(): void
    {
        $wanted = $this->interaction(['task_id' => 'ABC-123']);
        $this->interaction(['task_id' => 'XYZ-9']);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage/task/ABC-123')->assertOk();
        $props = $response->viewData('page')['props'];

        $this->assertSame('ABC-123', $props['task']);
        $ids = collect($props['interactions'])->pluck('id')->all();
        $this->assertContains($wanted->id, $ids);
        $this->assertCount(1, $ids);
    }

    public function test_the_untagged_bucket_has_its_own_task_page(): void
    {
        $untagged = $this->interaction(['task_id' => null]);
        $this->interaction(['task_id' => 'ABC-123']);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage/task/untagged')->assertOk();
        $props = $response->viewData('page')['props'];

        $this->assertTrue($props['untagged']);
        $this->assertSame([$untagged->id], collect($props['interactions'])->pluck('id')->all());
    }

    public function test_a_member_cannot_open_a_task_page(): void
    {
        $this->interaction(['task_id' => 'ABC-123']);

        $this->actingAs($this->user())->get('/usage/task/ABC-123')->assertForbidden();
    }

    public function test_the_dashboard_carries_a_recent_list_with_ids_to_open(): void
    {
        $interaction = $this->interaction(['task_id' => 'ABC-123']);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->assertOk();
        $recent = collect($response->viewData('page')['props']['recent']);

        $this->assertTrue($recent->contains(fn ($row) => $row['id'] === $interaction->id));
    }

    public function test_the_audit_log_lists_who_read_what(): void
    {
        $subject = $this->user();
        $interaction = $this->interaction(['user_id' => $subject->id]);
        $admin = $this->user(User::ROLE_ADMIN, raw: true);

        $this->actingAs($admin)->get("/usage/{$interaction->id}/raw?reason=checking")->assertOk();

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage/audit')->assertOk();
        $views = $response->viewData('page')['props']['views'];

        $this->assertCount(1, $views);
        $this->assertSame($admin->name, $views[0]['actor']);
        $this->assertSame('checking', $views[0]['reason']);
    }

    public function test_scores_are_shown_with_their_reasons(): void
    {
        $interaction = $this->interaction();

        QualityScore::withoutGlobalScope('tenant')->create([
            'tenant_id' => $this->tenant->id,
            'ai_interaction_id' => $interaction->id,
            'rubric_version' => 1,
            'score' => 55,
            'dimensions' => ['clear_goal' => ['score' => 30, 'reason' => 'No clear action.']],
            'reasons' => ['No clear action.'],
        ]);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->assertOk();
        $weakest = $response->viewData('page')['props']['weakest'];

        $this->assertSame('clear_goal', $weakest[0]['dimension']);
        $this->assertSame('No clear action.', $weakest[0]['common_reason']);
    }
}
