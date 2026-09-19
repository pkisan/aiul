<?php

namespace Tests\Feature;

use App\Models\AiInteraction;
use App\Models\Device;
use App\Models\QualityScore;
use App\Models\Tenant;
use App\Services\BodyStore;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Storage;
use Tests\TestCase;

/**
 * Retention is the promise the module rests on: the words go, the numbers stay.
 * These tests are the ones that fail if it ever stops being true.
 */
class RetentionTest extends TestCase
{
    use RefreshDatabase;

    protected function setUp(): void
    {
        parent::setUp();
        Storage::fake('s3');
        app(TenantContext::class)->set(null);
    }

    private function tenant(string $slug, int $retentionDays): Tenant
    {
        return Tenant::create([
            'name' => ucfirst($slug),
            'slug' => $slug,
            'retention_days' => $retentionDays,
        ]);
    }

    /** One interaction with real bodies stored, at a chosen age. */
    private function interaction(Tenant $tenant, int $daysAgo, string $prompt = 'refactor the login form'): AiInteraction
    {
        $bodies = app(BodyStore::class);

        [, $hash] = Device::issueToken();
        $device = Device::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id,
            'hostname' => 'mac',
            'token_hash' => $hash,
        ]);

        $eventId = BodyStore::newEventId();

        return AiInteraction::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id,
            'device_id' => $device->id,
            'event_id' => $eventId,
            'host' => 'api.anthropic.com',
            'task_id' => 'AIUL-7',
            'prompt_object' => $bodies->put($tenant->id, $eventId, 'prompt', $prompt),
            'answer_object' => $bodies->put($tenant->id, $eventId, 'answer', 'here is the diff'),
            'prompt_chars' => strlen($prompt),
            'prompt_tokens' => 40,
            'response_tokens' => 120,
            'duration_ms' => 2500,
            'occurred_at' => now()->subDays($daysAgo),
        ]);
    }

    public function test_bodies_past_the_window_are_deleted_and_the_metrics_survive(): void
    {
        $tenant = $this->tenant('acme', 30);
        $old = $this->interaction($tenant, daysAgo: 45);

        QualityScore::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id,
            'ai_interaction_id' => $old->id,
            'score' => 82,
            'rubric_version' => 1,
            'dimensions' => ['clear_goal' => ['score' => 10, 'reason' => 'names the file']],
        ]);

        $promptKey = $old->prompt_object;
        $answerKey = $old->answer_object;
        Storage::disk('s3')->assertExists($promptKey);

        $this->artisan('aiul:purge-bodies')->assertSuccessful();

        Storage::disk('s3')->assertMissing($promptKey);
        Storage::disk('s3')->assertMissing($answerKey);

        $old->refresh();
        $this->assertNull($old->prompt_object);
        $this->assertNull($old->answer_object);

        // Everything the dashboard reports is still here.
        $this->assertSame('AIUL-7', $old->task_id);
        $this->assertSame(40, $old->prompt_tokens);
        $this->assertSame(2500, $old->duration_ms);
        $this->assertSame(23, $old->prompt_chars);
        $this->assertSame(82, $old->score->score);
    }

    public function test_bodies_inside_the_window_are_left_alone(): void
    {
        $tenant = $this->tenant('acme', 30);
        $recent = $this->interaction($tenant, daysAgo: 5);

        $this->artisan('aiul:purge-bodies')->assertSuccessful();

        Storage::disk('s3')->assertExists($recent->prompt_object);
        $this->assertNotNull($recent->refresh()->prompt_object);
    }

    /** Retention is per customer, so one tenant's window must not purge another's. */
    public function test_each_tenant_gets_its_own_window(): void
    {
        $short = $this->tenant('short', 7);
        $long = $this->tenant('long', 365);

        $shortLived = $this->interaction($short, daysAgo: 30);
        $longLived = $this->interaction($long, daysAgo: 30);

        $this->artisan('aiul:purge-bodies')->assertSuccessful();

        $this->assertNull($shortLived->refresh()->prompt_object);
        $this->assertNotNull($longLived->refresh()->prompt_object);
        Storage::disk('s3')->assertExists($longLived->answer_object);
    }

    public function test_a_dry_run_changes_nothing(): void
    {
        $tenant = $this->tenant('acme', 30);
        $old = $this->interaction($tenant, daysAgo: 45);

        $this->artisan('aiul:purge-bodies --dry-run')->assertSuccessful();

        Storage::disk('s3')->assertExists($old->prompt_object);
        $this->assertNotNull($old->refresh()->prompt_object);
    }

    public function test_running_it_twice_is_safe(): void
    {
        $tenant = $this->tenant('acme', 30);
        $old = $this->interaction($tenant, daysAgo: 45);

        $this->artisan('aiul:purge-bodies')->assertSuccessful();
        $this->artisan('aiul:purge-bodies')->assertSuccessful();

        $this->assertNull($old->refresh()->prompt_object);
    }

    /**
     * A purged interaction must read as "no text", not as an error. The raw-prompt
     * view and the "my data" page both go through BodyStore::get.
     */
    public function test_reading_a_purged_body_returns_null_rather_than_failing(): void
    {
        $tenant = $this->tenant('acme', 30);
        $old = $this->interaction($tenant, daysAgo: 45);
        $key = $old->prompt_object;

        $this->artisan('aiul:purge-bodies')->assertSuccessful();

        $this->assertNull(app(BodyStore::class)->get($key));
    }
}
