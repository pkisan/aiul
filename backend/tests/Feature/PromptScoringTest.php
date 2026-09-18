<?php

namespace Tests\Feature;

use App\Jobs\ScoreInteraction;
use App\Models\AiInteraction;
use App\Models\Device;
use App\Models\QualityScore;
use App\Models\Tenant;
use App\Services\BodyStore;
use App\Services\PromptScorer;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Storage;
use Tests\TestCase;

class PromptScoringTest extends TestCase
{
    use RefreshDatabase;

    protected function setUp(): void
    {
        parent::setUp();
        Storage::fake('s3');
        app(TenantContext::class)->set(null);
    }

    private function scorer(): PromptScorer
    {
        return new PromptScorer;
    }

    public function test_a_thorough_prompt_scores_higher_than_a_vague_one(): void
    {
        $vague = $this->scorer()->score('fix the bug');

        $thorough = $this->scorer()->score(<<<'PROMPT'
        Refactor the UserController@store method to use a form request.
        The file is app/Http/Controllers/UserController.php and it currently fails
        with "Undefined array key email" when the request has no email.
        You must keep the existing validation rules and you should not change the
        route. Return only the diff, for example:

        ```diff
        - public function store(Request $request)
        + public function store(StoreUserRequest $request)
        ```
        PROMPT);

        $this->assertGreaterThan($vague['score'], $thorough['score']);
        $this->assertGreaterThan(70, $thorough['score']);
        $this->assertLessThan(60, $vague['score']);
    }

    public function test_every_score_carries_its_six_dimensions_and_reasons(): void
    {
        $result = $this->scorer()->score('Explain what a TLS proxy does in two sentences.');

        $this->assertSame(PromptScorer::RUBRIC_VERSION, $result['version']);
        $this->assertEqualsCanonicalizing(
            ['clear_goal', 'context_given', 'constraints_stated', 'expected_output', 'examples', 'focus'],
            array_keys($result['dimensions'])
        );

        // Every dimension explains itself in plain English — a score nobody can
        // question is a score nobody trusts.
        foreach ($result['dimensions'] as $name => $dimension) {
            $this->assertArrayHasKey('score', $dimension, $name);
            $this->assertArrayHasKey('reason', $dimension, $name);
            $this->assertNotEmpty($dimension['reason'], $name);
            $this->assertGreaterThanOrEqual(0, $dimension['score']);
            $this->assertLessThanOrEqual(100, $dimension['score']);
        }
    }

    public function test_an_empty_prompt_scores_zero_without_failing(): void
    {
        $result = $this->scorer()->score('   ');

        $this->assertSame(0, $result['score']);
        $this->assertNotEmpty($result['reasons']);
    }

    public function test_the_score_stays_within_range_for_odd_input(): void
    {
        foreach (['?', str_repeat('a', 20000), "\n\n\n", '🙂', '<?php echo 1;'] as $prompt) {
            $result = $this->scorer()->score($prompt);

            $this->assertGreaterThanOrEqual(0, $result['score']);
            $this->assertLessThanOrEqual(100, $result['score']);
        }
    }

    public function test_the_job_scores_a_stored_interaction(): void
    {
        $interaction = $this->storedInteraction(
            'Refactor this to use a form request. Must keep the validation. Return only the diff.'
        );

        (new ScoreInteraction($interaction->id, $interaction->tenant_id))->handle(
            app(BodyStore::class), $this->scorer(), app(TenantContext::class)
        );

        $score = QualityScore::withoutGlobalScope('tenant')->first();

        $this->assertNotNull($score);
        $this->assertSame($interaction->id, $score->ai_interaction_id);
        $this->assertSame(PromptScorer::RUBRIC_VERSION, $score->rubric_version);
        $this->assertGreaterThan(0, $score->score);
        $this->assertNotEmpty($score->dimensions);
        $this->assertNotEmpty($score->reasons);
    }

    public function test_an_automated_follow_up_is_not_scored(): void
    {
        // These are the agent talking to itself. Scoring them as if a person wrote
        // them would drag every average down unfairly.
        $interaction = $this->storedInteraction('tool result: 42', ['automated' => true]);

        (new ScoreInteraction($interaction->id, $interaction->tenant_id))->handle(
            app(BodyStore::class), $this->scorer(), app(TenantContext::class)
        );

        $this->assertSame(0, QualityScore::withoutGlobalScope('tenant')->count());
    }

    public function test_running_the_job_twice_does_not_create_two_scores(): void
    {
        $interaction = $this->storedInteraction('Explain this function and return a summary.');

        foreach (range(1, 2) as $ignored) {
            (new ScoreInteraction($interaction->id, $interaction->tenant_id))->handle(
                app(BodyStore::class), $this->scorer(), app(TenantContext::class)
            );
        }

        $this->assertSame(1, QualityScore::withoutGlobalScope('tenant')->count());
    }

    public function test_a_purged_body_is_skipped_rather_than_failing(): void
    {
        // Retention deletes bodies and keeps the metrics, so this is a normal state.
        $interaction = $this->storedInteraction('something');
        Storage::disk('s3')->delete($interaction->prompt_object);

        (new ScoreInteraction($interaction->id, $interaction->tenant_id))->handle(
            app(BodyStore::class), $this->scorer(), app(TenantContext::class)
        );

        $this->assertSame(0, QualityScore::withoutGlobalScope('tenant')->count());
    }

    private function storedInteraction(string $prompt, array $overrides = []): AiInteraction
    {
        $tenant = Tenant::create(['name' => 'Acme', 'slug' => 'acme']);
        [, $hash] = Device::issueToken();
        $device = Device::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id,
            'hostname' => 'mac',
            'token_hash' => $hash,
        ]);

        $eventId = bin2hex(random_bytes(12));
        $key = app(BodyStore::class)->put($tenant->id, $eventId, 'prompt', $prompt);

        return AiInteraction::withoutGlobalScope('tenant')->create(array_merge([
            'tenant_id' => $tenant->id,
            'device_id' => $device->id,
            'event_id' => $eventId,
            'host' => 'api.anthropic.com',
            'prompt_object' => $key,
            'occurred_at' => now(),
        ], $overrides));
    }
}
