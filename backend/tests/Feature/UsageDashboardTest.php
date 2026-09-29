<?php

namespace Tests\Feature;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\Device;
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

    private function user(string $role = User::ROLE_MEMBER, bool $raw = false, ?Tenant $tenant = null, bool $consented = true): User
    {
        $user = User::create([
            'tenant_id' => ($tenant ?? $this->tenant)->id,
            'name' => ucfirst($role),
            'email' => $role.'-'.uniqid().'@example.com',
            'password' => 'password',
            'role' => $role,
            'can_view_raw_prompts' => $raw,
        ]);

        if ($consented) {
            ConsentRecord::withoutGlobalScope('tenant')->create([
                'tenant_id' => $user->tenant_id, 'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
                'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
            ]);
        }

        return $user;
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

    // The dashboard reports per project — the repository the work happened in.
    // Branch names here carry no ticket, which is the normal case and used to
    // mean nothing was reported at all.
    public function test_a_manager_sees_usage_per_project(): void
    {
        $this->interaction(['repo' => '/Users/dev/plrb-lms', 'branch' => 'feature/revised-wordpress-sso']);
        $this->interaction(['repo' => '/Users/dev/plrb-lms', 'branch' => 'main']);
        $this->interaction(['repo' => '/Users/dev/other']);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->assertOk();

        $perProject = collect($response->viewData('page')['props']['perProject']);
        $lms = $perProject->firstWhere('repo', '/Users/dev/plrb-lms');

        $this->assertSame('plrb-lms', $lms['name']);
        $this->assertSame(2, $lms['interactions']);
        $this->assertSame(2, $lms['prompts']);
        $this->assertTrue($perProject->contains(fn ($row) => $row['repo'] === '/Users/dev/other'));
    }

    // Every Overview number is read against the period before it.
    public function test_the_overview_compares_with_the_previous_period(): void
    {
        $this->interaction(['kind' => 'human', 'occurred_at' => now()->subDays(2)]);
        $this->interaction(['kind' => 'human', 'occurred_at' => now()->subDays(3)]);
        $this->interaction(['kind' => 'human', 'occurred_at' => now()->subDays(10)]); // previous 7 days
        $this->interaction(['kind' => 'human', 'occurred_at' => now()->subDays(20)]); // older: in neither

        $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')
            ->assertOk()
            ->assertInertia(fn ($page) => $page
                ->component('Usage/Overview')
                ->where('days', 7)
                ->where('summary.prompts', 2)
                ->where('previous.prompts', 1)
                ->where('insights.0', 'Prompts up 100% on the previous period.'));
    }

    // Empty days are bars of zero, not missing bars: a gap is information.
    public function test_the_activity_chart_has_every_bucket_in_the_period(): void
    {
        $this->interaction(['kind' => 'human', 'repo' => '/Users/dev/lms', 'occurred_at' => now()->subDays(2)]);
        $this->interaction(['kind' => 'agent', 'repo' => '/Users/dev/lms', 'occurred_at' => now()->subDays(2)]); // not a prompt

        $activity = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage?days=30')
            ->viewData('page')['props']['activity'];

        $this->assertSame('day', $activity['unit']);
        $this->assertCount(31, $activity['buckets']);
        $this->assertSame(1, array_sum($activity['series']['/Users/dev/lms']));
        $this->assertCount(31, $activity['series']['/Users/dev/lms']);
    }

    // How people work with AI, not which tool they picked.
    public function test_the_overview_describes_how_the_team_works(): void
    {
        $alex = $this->user();
        $first = $this->interaction(['user_id' => $alex->id, 'kind' => 'human', 'repo' => '/Users/dev/lms']);
        $this->interaction(['user_id' => $alex->id, 'kind' => 'agent', 'repo' => '/Users/dev/lms', 'ai_session_id' => $first->ai_session_id]);
        $this->interaction(['user_id' => $alex->id, 'kind' => 'agent', 'repo' => '/Users/dev/lms', 'ai_session_id' => $first->ai_session_id]);
        $this->interaction(['user_id' => $alex->id, 'kind' => 'human', 'repo' => null]);

        $props = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->viewData('page')['props'];

        $this->assertSame(50, $props['patterns']['project_share']);
        $this->assertEquals(1.0, $props['patterns']['steps_per_prompt']);
        $this->assertEquals(1.0, $props['patterns']['prompts_per_session']);
        $this->assertEquals(1.0, $props['patterns']['days_per_person']);

        $row = collect($props['team'])->firstWhere('user_id', $alex->id);
        $this->assertSame('lms', $row['main_project']);
        $this->assertSame(1, $row['active_days']);
        $this->assertSame(2, $row['agent_steps']);
    }

    public function test_the_overview_flags_secrets_and_quiet_devices(): void
    {
        $alex = $this->user();
        $this->device->forceFill(['user_id' => $alex->id, 'last_seen_at' => now()->subDays(5)])->save();
        [, $hash] = Device::issueToken();
        Device::withoutGlobalScope('tenant')->create([
            'tenant_id' => $this->tenant->id, 'hostname' => 'spare-mac', 'token_hash' => $hash, 'last_seen_at' => now(),
        ]);
        $this->interaction(['redacted' => ['aws-access-key']]);
        $this->interaction(['redacted' => []]); // nothing masked: not a secret

        $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')
            ->assertInertia(fn ($page) => $page
                ->where('attention.secrets_total', 1)
                ->where('attention.secrets.0.rules', ['aws-access-key'])
                ->where('attention.silent.0.hostname', 'mac')
                ->where('attention.unassigned.0.hostname', 'spare-mac')
                // Alex has a device but did nothing: still a row on the team.
                ->where('team', fn ($team) => collect($team)->contains(fn ($p) => $p['user_id'] === $alex->id && $p['prompts'] === 0 && $p['enrolled']))
                ->where('enrolled', 1));
    }

    public function test_the_overview_shows_only_the_managers_tenant(): void
    {
        $other = Tenant::create(['name' => 'Other', 'slug' => 'other']);
        [, $hash] = Device::issueToken();
        $theirs = Device::withoutGlobalScope('tenant')->create(['tenant_id' => $other->id, 'hostname' => 'their-mac', 'token_hash' => $hash]);
        $this->interaction(['tenant_id' => $other->id, 'device_id' => $theirs->id, 'kind' => 'human', 'redacted' => ['email']]);
        $this->interaction(['kind' => 'human']);

        $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')
            ->assertInertia(fn ($page) => $page
                ->where('summary.prompts', 1)
                ->where('attention.secrets_total', 0)
                ->where('attention.unassigned', fn ($d) => collect($d)->pluck('hostname')->all() === ['mac'])
                ->has('latest', 1));
    }

    // The tenant comes from the signed-in user, who is read from the session,
    // so the middleware must run after StartSession. Run earlier, the user is
    // unknown, no tenant is set, and every tenant's rows show. It must still
    // run before SubstituteBindings, or another tenant's row is found and then
    // refused (a 403 that admits it exists) instead of not found.
    // An HTTP test cannot catch this: actingAs() and the in-memory test session
    // know the user before the session starts.
    public function test_the_tenant_is_set_after_the_session_and_before_route_binding(): void
    {
        $router = app('router');
        $route = $router->getRoutes()->match(\Illuminate\Http\Request::create('/usage'));
        $order = $router->resolveMiddleware($router->gatherRouteMiddleware($route));
        $at = fn ($class) => array_search($class, $order, true);

        $this->assertGreaterThan($at(\Illuminate\Session\Middleware\StartSession::class), $at(\App\Http\Middleware\SetTenantFromUser::class));
        $this->assertLessThan($at(\Illuminate\Routing\Middleware\SubstituteBindings::class), $at(\App\Http\Middleware\SetTenantFromUser::class));
    }

    public function test_a_member_cannot_open_the_activity_list(): void
    {
        $this->actingAs($this->user())->get('/usage/activity')->assertForbidden();
    }

    public function test_work_outside_a_checkout_gets_its_own_row_rather_than_disappearing(): void
    {
        $this->interaction(['repo' => null]);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage')->assertOk();
        $unknown = collect($response->viewData('page')['props']['perProject'])->firstWhere('repo', null);

        $this->assertNotNull($unknown, 'work outside a checkout must appear as its own row');
        $this->assertSame(1, $unknown['interactions']);
    }

    public function test_a_project_page_lists_only_that_projects_interactions(): void
    {
        $mine = $this->interaction(['repo' => '/Users/dev/plrb-lms']);
        $this->interaction(['repo' => '/Users/dev/other']);

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/project?repo='.urlencode('/Users/dev/plrb-lms'))
            ->assertOk()
            ->assertInertia(fn ($page) => $page
                ->component('Usage/Project')
                ->where('name', 'plrb-lms')
                ->has('interactions.data', 1)
                ->where('interactions.data.0.id', $mine->id));
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
            ->get("/usage/{$interaction->id}")
            ->assertOk()
            ->assertInertia(fn ($page) => $page->missing('prompt')->where('canViewRaw', false));

    }

    public function test_an_admin_without_the_flag_still_cannot_read_prompt_text(): void
    {
        $interaction = $this->interaction();

        $this->actingAs($this->user(User::ROLE_ADMIN, raw: false))
            ->get("/usage/{$interaction->id}")
            ->assertOk()
            ->assertInertia(fn ($page) => $page->missing('prompt'));
    }

    public function test_anyone_may_read_their_own_prompt(): void
    {
        $member = $this->user();
        $interaction = $this->interaction(['user_id' => $member->id], 'my own words');

        $response = $this->actingAs($member)->get("/usage/{$interaction->id}")->assertOk();

        $this->assertSame('my own words', $response->viewData('page')['props']['prompt']);
    }

    public function test_nobody_can_reach_another_tenants_interaction(): void
    {
        $other = Tenant::create(['name' => 'Globex', 'slug' => 'globex']);
        $interaction = $this->interaction();

        $outsider = $this->user(User::ROLE_ADMIN, raw: true, tenant: $other);

        // Not 403 but 404: the row is invisible to them, which is what the global
        // scope means.
        $this->actingAs($outsider)->get("/usage/{$interaction->id}")->assertNotFound();

    }

    public function test_my_data_shows_what_was_captured(): void
    {
        $member = $this->user();
        $this->interaction(['user_id' => $member->id]);

        $response = $this->actingAs($member)->get('/my-data')->assertOk();
        $props = $response->viewData('page')['props'];

        $this->assertSame(1, $props['summary']['total']);
    }

    public function test_my_data_is_open_to_every_role(): void
    {
        $this->actingAs($this->user())->get('/my-data')->assertOk();
        $this->actingAs($this->user(User::ROLE_MANAGER))->get('/my-data')->assertOk();
    }

    public function test_the_prompt_list_paginates_twenty_five_at_a_time(): void
    {
        for ($n = 0; $n < 30; $n++) {
            $this->interaction(['kind' => 'human']);
        }
        $this->interaction(['kind' => 'agent']); // not a prompt: never listed

        $page1 = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage/activity?tool=claude-code')->assertOk();
        $pager = $page1->viewData('page')['props']['list'];

        $this->assertSame(30, $pager['total']);
        $this->assertCount(25, $pager['data']);
        // The filters ride along on every page link.
        $this->assertStringContainsString('tool=claude-code', $pager['next_page_url']);

        $page2 = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage/activity?page=2')->assertOk();
        $pager2 = $page2->viewData('page')['props']['list'];

        $this->assertSame(2, $pager2['current_page']);
        $this->assertCount(5, $pager2['data']);
    }

    public function test_the_prompt_list_carries_ids_to_open(): void
    {
        $interaction = $this->interaction(['task_id' => 'ABC-123']);

        $response = $this->actingAs($this->user(User::ROLE_MANAGER))->get('/usage/activity')->assertOk();
        $row = $response->viewData('page')['props']['list']['data'][0];

        $this->assertSame($interaction->id, $row['id']);
        $this->assertSame($interaction->ai_session_id, $row['session_id']);
        $this->assertNull($row['preview'], 'a manager without the grant sees no prompt text');
        $this->assertArrayNotHasKey('prompt_object', $row, 'storage keys stay on the server');
    }

    public function test_a_missing_body_says_empty_when_nothing_was_captured(): void
    {
        // An exchange with no text stores no body at all — the page must say
        // that, not blame the retention policy.
        $interaction = $this->interaction();
        $interaction->forceFill([
            'prompt_object' => null, 'answer_object' => null,
            'prompt_chars' => 0, 'answer_chars' => 0,
        ])->save();

        $response = $this->actingAs($this->user(User::ROLE_ADMIN, raw: true))
            ->get("/usage/{$interaction->id}")
            ->assertOk();

        $props = $response->viewData('page')['props'];
        $this->assertSame('empty', $props['promptState']);
        $this->assertSame('empty', $props['answerState']);
    }

    public function test_a_missing_body_says_purged_when_chars_were_recorded(): void
    {
        $interaction = $this->interaction([], 'a prompt about pineapples');
        $interaction->forceFill([
            'prompt_object' => null, 'answer_object' => null,
            'prompt_chars' => 25, 'answer_chars' => 10,
        ])->save();

        $response = $this->actingAs($this->user(User::ROLE_ADMIN, raw: true))
            ->get("/usage/{$interaction->id}")
            ->assertOk();

        $props = $response->viewData('page')['props'];
        $this->assertSame('purged', $props['promptState']);
        $this->assertSame('purged', $props['answerState']);
    }

    // One message to an agent produces a dozen interactions, so the dashboard
    // leads with sessions — the stretch of work as it actually happened.
    public function test_the_dashboard_groups_work_into_sessions(): void
    {
        $first = $this->interaction();
        // A second interaction in the SAME session: an agent's follow-up, which
        // must not be counted as another human prompt.
        $this->interaction(['ai_session_id' => $first->ai_session_id, 'automated' => true]);

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/activity')
            ->assertOk()
            ->assertInertia(fn ($page) => $page->has('list.data'));

        $sessions = collect((new \App\Services\UsageReport(30))->sessions()->items())
            ->keyBy('id');

        $this->assertSame(1, $sessions[$first->ai_session_id]['human_prompts']);
    }

    // A session still being worked in started hours ago. Ordering by start time
    // buried the one the reader is actually in under every session opened since.
    public function test_sessions_are_ordered_by_last_activity(): void
    {
        $old = $this->interaction();
        AiSession::withoutGlobalScope('tenant')->where('id', $old->ai_session_id)->update([
            'started_at' => now()->subHours(4),
            'ended_at' => now()->subMinute(),   // started long ago, still going
        ]);

        $recent = $this->interaction();
        AiSession::withoutGlobalScope('tenant')->where('id', $recent->ai_session_id)->update([
            'started_at' => now()->subMinutes(30),
            'ended_at' => now()->subMinutes(20),  // began later, finished earlier
        ]);

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/activity?view=sessions')
            ->assertOk()
            ->assertInertia(fn ($page) => $page
                ->where('list.data.0.id', $old->ai_session_id));
    }

    public function test_a_session_reads_forwards_and_lists_its_interactions(): void
    {
        $interaction = $this->interaction();

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/session/'.$interaction->ai_session_id)
            ->assertOk()
            ->assertInertia(fn ($page) => $page
                ->component('Usage/Session')
                ->where('session.id', $interaction->ai_session_id)
                ->has('interactions', 1)
                ->where('interactions.0.id', $interaction->id));
    }

    // Session 19 was forty rows of which two were the owner's. A turn is one
    // message they typed, every request the agent made about it, and the reply.
    public function test_a_session_groups_interactions_into_turns(): void
    {
        $asked = $this->interaction(['kind' => 'human', 'automated' => false, 'answer_chars' => 0,
            'occurred_at' => now()->subMinutes(9)]);
        $session = $asked->ai_session_id;

        $this->interaction(['ai_session_id' => $session, 'kind' => 'utility', 'automated' => true,
            'answer_chars' => 11, 'occurred_at' => now()->subMinutes(8)]);
        $this->interaction(['ai_session_id' => $session, 'kind' => 'agent', 'automated' => true,
            'answer_chars' => 0, 'occurred_at' => now()->subMinutes(7)]);
        $reply = $this->interaction(['ai_session_id' => $session, 'kind' => 'agent', 'automated' => true,
            'answer_chars' => 3051, 'occurred_at' => now()->subMinutes(6)]);

        $rows = collect((new \App\Services\UsageReport(30))
            ->interactionsForSession(AiSession::withoutGlobalScope('tenant')->find($session)))
            ->keyBy('id');

        // Everything after the person's message belongs to their turn.
        $this->assertSame(1, $rows[$asked->id]['turn']);
        $this->assertSame(1, $rows[$reply->id]['turn']);

        // The answer they read is the last row of the turn that carries text,
        // and the tool's own housekeeping can never be it.
        $this->assertTrue($rows[$reply->id]['final_answer']);
        $this->assertFalse($rows[$asked->id]['final_answer']);

        $utility = $rows->firstWhere('kind', 'utility');
        $this->assertFalse($utility['final_answer'], 'a utility call is not the answer to anything');
    }

    public function test_opening_a_session_shows_previews(): void
    {
        $asked = $this->interaction(['kind' => 'human', 'automated' => false], 'Fix the failing SSO tests.');
        $this->interaction(['ai_session_id' => $asked->ai_session_id, 'kind' => 'agent', 'automated' => true]);

        // The raw grant only means anything on an admin — see User::canViewRawPrompts.
        $this->actingAs($this->user(User::ROLE_ADMIN, raw: true))
            ->get('/usage/session/'.$asked->ai_session_id)
            ->assertOk()
            ->assertInertia(fn ($page) => $page
                ->where('interactions.0.prompt_preview', 'Fix the failing SSO tests.'));
    }

    public function test_a_manager_without_the_raw_grant_sees_no_prompt_text(): void
    {
        $asked = $this->interaction(['kind' => 'human'], 'Fix the failing SSO tests.');

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/session/'.$asked->ai_session_id)
            ->assertOk()
            ->assertInertia(fn ($page) => $page->where('interactions.0.prompt_preview', null));
    }

    public function test_a_member_cannot_open_a_session(): void
    {
        $interaction = $this->interaction();

        $this->actingAs($this->user())
            ->get('/usage/session/'.$interaction->ai_session_id)
            ->assertForbidden();
    }

    // Old links to the separate text page still land on the interaction.
    public function test_the_old_raw_link_redirects_to_the_interaction(): void
    {
        $interaction = $this->interaction();

        $this->actingAs($this->user(User::ROLE_ADMIN, raw: true))
            ->get("/usage/{$interaction->id}/raw")
            ->assertRedirect("/usage/{$interaction->id}");
    }

    public function test_the_interaction_page_shows_only_what_the_person_typed(): void
    {
        $member = $this->user();
        $interaction = $this->interaction(['user_id' => $member->id], "<system-reminder>\nCLAUDE.md\n</system-reminder>\nHi");

        $this->actingAs($member)->get("/usage/{$interaction->id}")
            ->assertInertia(fn ($page) => $page->where('prompt', 'Hi'));
    }

    public function test_the_dashboard_filters_by_person_and_tool(): void
    {
        $alex = $this->user();
        $this->interaction(['user_id' => $alex->id, 'tool' => 'cursor']);
        $this->interaction(['tool' => 'claude-code']);

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get("/usage/activity?person={$alex->id}")
            ->assertInertia(fn ($page) => $page
                ->where('summary.interactions', 1)
                ->where('filters.person', $alex->id)
                ->has('options.people'));

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/activity?tool=claude-code')
            ->assertInertia(fn ($page) => $page->where('summary.interactions', 1));

        // "-" is work outside any checkout; a path is one project.
        $this->interaction(['repo' => '/Users/dev/plrb-lms']);
        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/activity?project='.urlencode('/Users/dev/plrb-lms'))
            ->assertInertia(fn ($page) => $page->where('summary.interactions', 1));
        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->get('/usage/activity?project=-')
            ->assertInertia(fn ($page) => $page->where('summary.interactions', 2));
    }

    // A preview is prompt text: shown only with the grant.
    public function test_prompt_previews_need_the_grant(): void
    {
        $alex = $this->user();
        $this->interaction(['user_id' => $alex->id, 'kind' => 'human'], "Fix\n  the   SSO tests.");
        $this->interaction(['user_id' => $alex->id, 'kind' => 'human']);

        $this->actingAs($this->user(User::ROLE_ADMIN, raw: true))
            ->get('/usage/activity')
            ->assertInertia(fn ($page) => $page->where('list.data.1.preview', 'Fix the SSO tests.'));
    }

    // There is no landing page: "/" sends each person to where they work.
    public function test_home_sends_each_role_to_its_own_page(): void
    {
        $this->get('/')->assertRedirect('/login');
        $this->get('/register')->assertNotFound(); // accounts are made by the admin
        $this->get('/login')->assertHeader('X-Frame-Options', 'DENY')->assertHeader('X-Content-Type-Options', 'nosniff');
        $this->actingAs($this->user())->get('/')->assertRedirect('/my-data');
        $this->actingAs($this->user(User::ROLE_MANAGER))->get('/')->assertRedirect('/usage');
    }

    public function test_first_sign_in_asks_for_consent_and_records_it(): void
    {
        $member = $this->user(consented: false);

        $this->actingAs($member)->get('/my-data')->assertRedirect('/consent');
        $this->actingAs($member)->post('/consent', [])->assertSessionHasErrors('accept');
        $this->actingAs($member)->post('/consent', ['accept' => true])->assertRedirect();

        $this->assertTrue($member->fresh()->hasConsented());
        $this->actingAs($member)->get('/my-data')->assertOk();
    }

    public function test_old_agent_written_prompts_can_be_reclassified(): void
    {
        $suggestion = $this->interaction(['kind' => 'human'], '[SUGGESTION MODE: Suggest what the user might type next');
        $real = $this->interaction(['kind' => 'human'], 'Fix the login bug.');

        $this->artisan('aiul:reclassify-prompts')->assertSuccessful();

        $this->assertSame('utility', $suggestion->fresh()->kind);
        $this->assertTrue($suggestion->fresh()->automated);
        $this->assertSame('human', $real->fresh()->kind);
    }
}
