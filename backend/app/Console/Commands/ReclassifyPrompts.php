<?php

namespace App\Console\Commands;

use App\Models\AiInteraction;
use App\Services\BodyStore;
use App\Services\PromptText;
use Illuminate\Console\Command;

/**
 * One-off repair for rows stored before ingestion recognised prompts the tool
 * wrote itself (Claude Code's next-prompt suggestion and its recap after a
 * break). Those were stored as human prompts, so they inflated every "prompts"
 * count. New rows are classified at ingestion; this fixes the old ones.
 *
 * Safe to run again: it only touches rows still marked human.
 */
class ReclassifyPrompts extends Command
{
    protected $signature = 'aiul:reclassify-prompts {--dry-run : Report what would change and change nothing}';

    protected $description = 'Mark stored agent-written "prompts" as the tool\'s own calls';

    public function handle(BodyStore $bodies): int
    {
        $changed = 0;

        AiInteraction::withoutGlobalScope('tenant')
            ->where(fn ($q) => $q->where('kind', 'human')->orWhere(fn ($q) => $q->whereNull('kind')->where('automated', false)))
            ->whereNotNull('prompt_object')
            ->chunkById(200, function ($rows) use ($bodies, &$changed) {
                foreach ($rows as $row) {
                    if (! PromptText::isToolGenerated(PromptText::clean($bodies->get($row->prompt_object)))) {
                        continue;
                    }

                    $changed++;
                    if (! $this->option('dry-run')) {
                        $row->forceFill(['kind' => 'utility', 'automated' => true])->save();
                    }
                }
            });

        $this->info(($this->option('dry-run') ? 'Would reclassify ' : 'Reclassified ').$changed.' rows.');

        return self::SUCCESS;
    }
}
