<?php

namespace App\Console\Commands;

use App\Models\AiInteraction;
use App\Models\Tenant;
use App\Services\BodyStore;
use Illuminate\Console\Command;

/**
 * Retention: delete the prompt and answer text once it is older than the tenant's
 * retention_days, and keep everything derived from it.
 *
 * This is the promise the whole module rests on — "we keep the metrics, not your
 * words" — so it is a scheduled job rather than a policy in a document. The rows
 * stay: the score, the task, the timings and the token counts are what the
 * dashboard shows, and none of them need the text.
 *
 * Deletion order matters. The object goes first and the row is cleared second, so
 * an interruption leaves a key pointing at a missing object — which BodyStore::get
 * already treats as purged — rather than a row that claims nothing is stored while
 * the text is still sitting in the bucket.
 */
class PurgeBodies extends Command
{
    protected $signature = 'aiul:purge-bodies
        {--tenant= : Only this tenant, by slug}
        {--dry-run : Report what would be purged and change nothing}';

    protected $description = 'Delete prompt and answer bodies past their tenant\'s retention window';

    public function handle(BodyStore $bodies): int
    {
        $dryRun = (bool) $this->option('dry-run');

        $tenants = Tenant::query()
            ->when($this->option('tenant'), fn ($query, $slug) => $query->where('slug', $slug))
            ->get();

        if ($tenants->isEmpty()) {
            $this->error('No matching tenant.');

            return self::FAILURE;
        }

        $purged = 0;

        foreach ($tenants as $tenant) {
            $cutoff = now()->subDays($tenant->retention_days);

            // withoutGlobalScope: this command runs with no current tenant, and it
            // deliberately walks every tenant one at a time with an explicit
            // where. Being explicit here means the query says what it does.
            $query = AiInteraction::withoutGlobalScope('tenant')
                ->where('tenant_id', $tenant->id)
                ->where('occurred_at', '<', $cutoff)
                ->where(function ($query) {
                    $query->whereNotNull('prompt_object')
                        ->orWhereNotNull('answer_object');
                });

            $count = (clone $query)->count();

            $this->line(sprintf(
                '%s: retention %d days, cutoff %s, %d interaction(s) still holding text',
                $tenant->slug,
                $tenant->retention_days,
                $cutoff->toDateTimeString(),
                $count,
            ));

            if ($dryRun || $count === 0) {
                continue;
            }

            // Chunked by id so a large backlog cannot exhaust memory, and so the
            // shifting result set of an update-as-we-go cannot skip rows.
            $query->chunkById(200, function ($interactions) use ($bodies, &$purged) {
                foreach ($interactions as $interaction) {
                    $bodies->delete($interaction->prompt_object);
                    $bodies->delete($interaction->answer_object);

                    $interaction->forceFill([
                        'prompt_object' => null,
                        'answer_object' => null,
                    ])->save();

                    $purged++;
                }
            });
        }

        $this->info($dryRun
            ? 'Dry run. Nothing was changed.'
            : sprintf('Purged the bodies of %d interaction(s).', $purged));

        return self::SUCCESS;
    }
}
