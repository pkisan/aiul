<?php

namespace Tests\Feature;

use App\Jobs\ScoreInteraction;
use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\Device;
use App\Models\Tenant;
use App\Services\BodyStore;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Queue;
use Illuminate\Support\Facades\Storage;
use Tests\TestCase;

class EventIngestionTest extends TestCase
{
    use RefreshDatabase;

    protected function setUp(): void
    {
        parent::setUp();

        // A fake disk: these tests exercise our code, not MinIO's.
        Storage::fake('s3');
        app(TenantContext::class)->set(null);
    }

    /** Registers a tenant and device, returning [device, plain token]. */
    private function newDevice(string $slug = 'acme', bool $linked = true): array
    {
        $tenant = Tenant::create(['name' => ucfirst($slug), 'slug' => $slug]);
        [$plain, $hash] = Device::issueToken();

        $user = null;
        if ($linked) {
            // A device linked by `aiul login` to someone who accepted the notice.
            $user = \App\Models\User::create([
                'tenant_id' => $tenant->id, 'name' => 'Alex', 'email' => $slug.'@example.com',
                'password' => 'password', 'role' => 'member',
            ]);
            \App\Models\ConsentRecord::withoutGlobalScope('tenant')->create([
                'tenant_id' => $tenant->id, 'user_id' => $user->id, 'kind' => 'capture',
                'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
            ]);
        }

        $device = Device::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id,
            'user_id' => $user?->id,
            'hostname' => $slug.'-macbook',
            'token_hash' => $hash,
        ]);

        return [$device, $plain];
    }

    public function test_events_from_an_unlinked_device_are_confirmed_and_discarded(): void
    {
        [, $token] = $this->newDevice(linked: false);
        $event = $this->anEvent();

        $this->withToken($token)->postJson('/api/aiul/events', ['events' => [$event]])
            ->assertOk()
            ->assertJsonPath('accepted.0', $event['id'])
            ->assertJsonStructure(['discarded']);

        $this->assertSame(0, AiInteraction::withoutGlobalScope('tenant')->count());
    }

    public function test_aiul_login_pairs_the_device_with_whoever_enters_the_code(): void
    {
        [$device, $token] = $this->newDevice(linked: false);

        $code = $this->withToken($token)->postJson('/api/aiul/pair')->assertOk()->json('code');
        $this->withToken($token)->getJson('/api/aiul/pair')->assertJsonPath('paired', false);

        $person = \App\Models\User::create([
            'tenant_id' => $device->tenant_id, 'name' => 'Alex John', 'email' => 'alex@example.com',
            'password' => 'password', 'role' => 'member',
        ]);
        \App\Models\ConsentRecord::withoutGlobalScope('tenant')->create([
            'tenant_id' => $device->tenant_id, 'user_id' => $person->id, 'kind' => 'capture',
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);

        $this->actingAs($person)->post('/pair', ['code' => strtolower(str_replace('-', '', $code))])
            ->assertSessionHasNoErrors();

        $this->withToken($token)->getJson('/api/aiul/pair')
            ->assertJsonPath('paired', true)
            ->assertJsonPath('name', 'Alex John')
            ->assertJsonPath('consented', true);

        // A used code cannot be entered again.
        $this->actingAs($person)->post('/pair', ['code' => $code])->assertSessionHasErrors('code');
    }

    private function anEvent(array $overrides = []): array
    {
        return array_merge([
            'schema' => 1,
            'id' => bin2hex(random_bytes(12)),
            'time' => now()->toIso8601String(),
            'host' => 'api.anthropic.com',
            'method' => 'POST',
            'path' => '/v1/messages',
            'status' => 200,
            'tool' => 'claude-code',
            'model' => 'claude-opus-5',
            'parser' => 'anthropic',
            'prompt' => 'Refactor the UserController to use a form request. Must keep the existing validation rules. Return only the diff.',
            'answer' => 'Here is the diff.',
            'task_id' => 'ABC-123',
            'repo' => '/Users/dev/project',
            'branch' => 'feature/ABC-123-forms',
            'streamed' => true,
            'prompt_tokens' => 24,
            'response_tokens' => 37,
            'duration_ms' => 2500,
            'allowlist_version' => 1,
            'redaction_rules_version' => 1,
            'redacted' => ['email'],
        ], $overrides);
    }

    public function test_a_request_with_no_token_is_rejected(): void
    {
        $this->postJson('/api/aiul/events', ['events' => [$this->anEvent()]])
            ->assertStatus(401);

        $this->assertSame(0, AiInteraction::withoutGlobalScope('tenant')->count());
    }

    public function test_an_unknown_token_is_rejected(): void
    {
        $this->withToken('aiul_not-a-real-token')
            ->postJson('/api/aiul/events', ['events' => [$this->anEvent()]])
            ->assertStatus(401);
    }

    public function test_a_revoked_device_is_rejected(): void
    {
        [$device, $token] = $this->newDevice();
        $device->forceFill(['revoked' => true])->save();

        $this->withToken($token)
            ->postJson('/api/aiul/events', ['events' => [$this->anEvent()]])
            ->assertStatus(401);
    }

    public function test_an_event_is_stored_and_its_id_reported_as_accepted(): void
    {
        Queue::fake();
        [$device, $token] = $this->newDevice();
        $event = $this->anEvent();

        $response = $this->withToken($token)
            ->postJson('/api/aiul/events', ['events' => [$event]])
            ->assertOk();

        // The agent deletes exactly what comes back here, so this contract matters.
        $this->assertSame([$event['id']], $response->json('accepted'));
        $this->assertSame([], $response->json('rejected'));

        $stored = AiInteraction::withoutGlobalScope('tenant')->first();

        $this->assertSame('api.anthropic.com', $stored->host);
        $this->assertSame('ABC-123', $stored->task_id);
        $this->assertSame('feature/ABC-123-forms', $stored->branch);
        $this->assertSame('claude-opus-5', $stored->model);
        $this->assertSame($device->tenant_id, $stored->tenant_id);
        $this->assertTrue($stored->streamed);
        $this->assertSame(['email'], $stored->redacted);
    }

    // The agent sends the device's local time with its offset. Stored without
    // converting, a Mac in IST wrote 15:29 into a UTC column and every screen
    // then showed 20:59 — the offset added a second time.
    public function test_a_device_in_another_timezone_is_stored_in_utc(): void
    {
        Queue::fake();
        [$device, $token] = $this->newDevice();

        $this->withToken($token)
            ->postJson('/api/aiul/events', ['events' => [
                $this->anEvent(['time' => '2026-09-21T15:29:42+05:30']),
            ]])
            ->assertOk();

        $stored = AiInteraction::withoutGlobalScope('tenant')->first();

        $this->assertSame('2026-09-21 09:59:42', $stored->occurred_at->utc()->format('Y-m-d H:i:s'));
    }

    public function test_prompt_text_is_kept_out_of_the_database_and_encrypted_in_object_storage(): void
    {
        Queue::fake();
        [, $token] = $this->newDevice();
        $event = $this->anEvent(['prompt' => 'a very specific prompt about pineapples']);

        $this->withToken($token)->postJson('/api/aiul/events', ['events' => [$event]])->assertOk();

        $stored = AiInteraction::withoutGlobalScope('tenant')->first();

        // The row holds a key, not the text.
        $this->assertIsString($stored->prompt_object);
        $this->assertArrayNotHasKey('prompt', $stored->getAttributes());

        // The stored object must not be readable as plain text.
        $raw = Storage::disk('s3')->get($stored->prompt_object);
        $this->assertStringNotContainsString('pineapples', $raw);

        // And our own reader gets it back.
        $this->assertSame(
            'a very specific prompt about pineapples',
            app(BodyStore::class)->get($stored->prompt_object)
        );
    }

    public function test_a_batch_sent_twice_does_not_create_duplicates(): void
    {
        Queue::fake();
        [, $token] = $this->newDevice();
        $event = $this->anEvent();

        $first = $this->withToken($token)->postJson('/api/aiul/events', ['events' => [$event]])->assertOk();
        $second = $this->withToken($token)->postJson('/api/aiul/events', ['events' => [$event]])->assertOk();

        // Both replies accept the id, so a retry after a timeout clears the spool.
        $this->assertSame([$event['id']], $first->json('accepted'));
        $this->assertSame([$event['id']], $second->json('accepted'));

        $this->assertSame(1, AiInteraction::withoutGlobalScope('tenant')->count());
    }

    public function test_one_malformed_event_does_not_lose_the_good_ones(): void
    {
        Queue::fake();
        [, $token] = $this->newDevice();

        $good = $this->anEvent();
        $bad = ['id' => 'broken', 'host' => '']; // no time, empty host

        $response = $this->withToken($token)
            ->postJson('/api/aiul/events', ['events' => [$bad, $good]])
            ->assertOk();

        $this->assertSame([$good['id']], $response->json('accepted'));
        $this->assertCount(1, $response->json('rejected'));
        $this->assertSame(1, AiInteraction::withoutGlobalScope('tenant')->count());
    }

    public function test_one_tenant_never_sees_another_tenants_rows(): void
    {
        Queue::fake();
        [$acme, $acmeToken] = $this->newDevice('acme');
        [$globex, $globexToken] = $this->newDevice('globex');

        $this->withToken($acmeToken)->postJson('/api/aiul/events', [
            'events' => [$this->anEvent(['task_id' => 'ACME-1'])],
        ])->assertOk();

        $this->withToken($globexToken)->postJson('/api/aiul/events', [
            'events' => [$this->anEvent(['task_id' => 'GLOBEX-1'])],
        ])->assertOk();

        $this->assertSame(2, AiInteraction::withoutGlobalScope('tenant')->count());

        // Reading as one tenant shows only that tenant's rows, with no `where`
        // written anywhere — that is the whole point of the global scope.
        app(TenantContext::class)->set($acme->tenant_id);
        $acmeRows = AiInteraction::all();
        $this->assertCount(1, $acmeRows);
        $this->assertSame('ACME-1', $acmeRows->first()->task_id);

        app(TenantContext::class)->set($globex->tenant_id);
        $globexRows = AiInteraction::all();
        $this->assertCount(1, $globexRows);
        $this->assertSame('GLOBEX-1', $globexRows->first()->task_id);
    }

    public function test_interactions_on_the_same_task_group_into_one_session(): void
    {
        Queue::fake();
        [, $token] = $this->newDevice();

        $this->withToken($token)->postJson('/api/aiul/events', [
            'events' => [
                $this->anEvent(['time' => now()->subMinutes(10)->toIso8601String()]),
                $this->anEvent(['time' => now()->subMinutes(5)->toIso8601String()]),
            ],
        ])->assertOk();

        $this->assertSame(1, AiSession::withoutGlobalScope('tenant')->count());
        $this->assertSame(2, AiSession::withoutGlobalScope('tenant')->first()->interaction_count);
    }

    // Sessions are grouped by the checkout. Two projects worked on inside the
    // same idle window are two sessions, even though neither carries a ticket —
    // grouping by task_id merged them, and a session then claimed interactions
    // from a repository it had nothing to do with.
    public function test_work_in_another_repository_starts_a_new_session(): void
    {
        Queue::fake();
        [, $token] = $this->newDevice();

        $this->withToken($token)->postJson('/api/aiul/events', [
            'events' => [
                $this->anEvent(['repo' => '/Users/dev/one', 'task_id' => null]),
                $this->anEvent(['repo' => '/Users/dev/two', 'task_id' => null]),
            ],
        ])->assertOk();

        $this->assertSame(2, AiSession::withoutGlobalScope('tenant')->count());
    }

    public function test_an_untagged_event_is_kept_rather_than_dropped(): void
    {
        Queue::fake();
        [, $token] = $this->newDevice();

        $this->withToken($token)->postJson('/api/aiul/events', [
            'events' => [$this->anEvent(['task_id' => null, 'branch' => 'main'])],
        ])->assertOk();

        $this->assertTrue(AiInteraction::withoutGlobalScope('tenant')->first()->isUntagged());
    }

    public function test_a_scoring_job_is_queued_for_each_new_interaction(): void
    {
        Queue::fake();
        [, $token] = $this->newDevice();

        $this->withToken($token)->postJson('/api/aiul/events', ['events' => [$this->anEvent()]])->assertOk();

        Queue::assertPushed(ScoreInteraction::class, 1);
    }

    public function test_the_ai_account_is_stored(): void
    {
        [, $token] = $this->newDevice();

        $this->withToken($token)->postJson('/api/aiul/events', ['events' => [$this->anEvent(['account' => 'alex@example.com'])]])
            ->assertOk();

        $this->assertSame('alex@example.com', AiInteraction::withoutGlobalScope('tenant')->value('account'));
    }
}
