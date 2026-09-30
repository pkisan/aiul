<?php

namespace Pm\Console;

use Illuminate\Console\Command;
use Pm\Linking\Linker;

/**
 * Links AI sessions to tasks. Scheduled every five minutes for sessions that
 * got new prompts recently; --days=N relinks everything started in the last N
 * days (after an import, or to backfill).
 */
class LinkCommand extends Command
{
    protected $signature = 'pm:link {--minutes=15 : sessions updated in the last N minutes} {--days= : instead, sessions started in the last N days}';

    protected $description = 'Link AI sessions to PM tasks';

    public function handle(Linker $linker): int
    {
        $count = $this->option('days')
            ? $linker->relink(startedSince: now()->subDays((int) $this->option('days')))
            : $linker->relink(since: now()->subMinutes((int) $this->option('minutes')));

        $this->info("Looked at {$count} sessions.");

        return self::SUCCESS;
    }
}
