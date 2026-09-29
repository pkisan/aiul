<?php

namespace App\Console\Commands;

use App\Models\AiInteraction;
use App\Services\UsageEraser;
use Illuminate\Console\Command;

/**
 * Empties Deleted prompts: anything deleted from the dashboard more than
 * config('aiul.trash_days') ago is removed for good, text included. Scheduled
 * nightly in routes/console.php.
 */
class PurgeDeleted extends Command
{
    protected $signature = 'aiul:purge-deleted {--dry-run : count only, change nothing}';

    protected $description = 'Permanently remove prompts deleted more than AIUL_TRASH_DAYS ago';

    public function handle(UsageEraser $eraser): int
    {
        $days = config('aiul.trash_days');

        // Every tenant: a command has no current tenant.
        $old = AiInteraction::withoutGlobalScope('tenant')->onlyTrashed()
            ->where('deleted_at', '<', now()->subDays($days))
            ->get(['id', 'tenant_id'])
            ->groupBy('tenant_id');

        $total = $old->flatten()->count();
        $this->line("{$total} interaction(s) deleted more than {$days} days ago.");

        if ($this->option('dry-run') || $total === 0) {
            return self::SUCCESS;
        }

        foreach ($old as $tenantId => $rows) {
            $eraser->delete((int) $tenantId, $rows->pluck('id'));
        }
        $this->info("Removed {$total} for good.");

        return self::SUCCESS;
    }
}
