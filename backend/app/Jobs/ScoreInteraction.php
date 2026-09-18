<?php

namespace App\Jobs;

use App\Models\AiInteraction;
use App\Models\QualityScore;
use App\Services\BodyStore;
use App\Services\PromptScorer;
use App\Support\TenantContext;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Queue\Queueable;

/**
 * Scores one interaction's prompt, on the queue.
 *
 * Queued for two reasons: ingestion has to stay fast, and a bug in scoring must
 * never cost us a captured event.
 */
class ScoreInteraction implements ShouldQueue
{
    use Queueable;

    public int $tries = 3;

    public function __construct(
        private readonly int $interactionId,
        private readonly int $tenantId,
    ) {}

    public function handle(BodyStore $bodies, PromptScorer $scorer, TenantContext $tenant): void
    {
        // A job has no request, so the tenant has to be set explicitly before any
        // query runs — otherwise the global scope has nothing to filter by.
        $tenant->runAs($this->tenantId, function () use ($bodies, $scorer) {
            $interaction = AiInteraction::find($this->interactionId);

            if (! $interaction) {
                return; // deleted between dispatch and running; nothing to do
            }

            // Automated follow-ups are the agent talking to itself. Scoring them
            // as if a person wrote them would drag every average down unfairly.
            if ($interaction->automated) {
                return;
            }

            $prompt = $bodies->get($interaction->prompt_object);

            if (blank($prompt)) {
                return;
            }

            $result = $scorer->score($prompt);

            QualityScore::updateOrCreate(
                [
                    'ai_interaction_id' => $interaction->id,
                    'rubric_version' => $result['version'],
                ],
                [
                    'tenant_id' => $interaction->tenant_id,
                    'score' => $result['score'],
                    'dimensions' => $result['dimensions'],
                    'reasons' => $result['reasons'],
                ]
            );
        });
    }
}
