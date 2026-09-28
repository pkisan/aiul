<?php

namespace App\Console\Commands;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\User;
use App\Services\BodyStore;
use Illuminate\Console\Command;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Support\Carbon;
use Illuminate\Support\Collection;

/**
 * Delete what was captured about one person: everything, a date range, or their
 * last N prompts. Their account, devices and consent stay; only usage goes.
 *
 *   php artisan aiul:delete-user-data kim@vardaam.com --all
 *   php artisan aiul:delete-user-data kim@vardaam.com --from=2026-09-01 --to=2026-09-15
 *   php artisan aiul:delete-user-data kim@vardaam.com --last=5
 *
 * A "prompt" means what the person typed, and deleting it takes the whole turn:
 * the agent's steps working on it and the answer it came back with. Deleting only
 * the typed line would leave the answer to a question nobody asked.
 *
 * Bodies go first, rows second (as in PurgeBodies): an interruption leaves a row
 * pointing at missing text, never text with no row to find it by.
 */
class DeleteUserData extends Command
{
    protected $signature = 'aiul:delete-user-data
        {email : the person whose data is deleted}
        {--all : everything captured about them}
        {--from= : from this date, YYYY-MM-DD (inclusive)}
        {--to= : up to this date, YYYY-MM-DD (inclusive)}
        {--last= : their last N prompts, with the steps and answers that belong to them}
        {--dry-run : show what would be deleted and change nothing}
        {--force : do not ask for confirmation}';

    protected $description = "Delete a person's captured AI usage: all of it, a date range, or their last N prompts";

    public function handle(BodyStore $bodies): int
    {
        $user = User::where('email', strtolower(trim($this->argument('email'))))->first();
        if (! $user) {
            $this->error("No user with email {$this->argument('email')}.");

            return self::FAILURE;
        }

        $modes = array_filter([
            'all' => $this->option('all'),
            'dates' => $this->option('from') || $this->option('to'),
            'last' => $this->option('last') !== null,
        ]);
        if (count($modes) !== 1) {
            $this->error('Choose exactly one of: --all, --from/--to, --last=N.');

            return self::FAILURE;
        }

        try {
            [$ids, $what] = match (array_key_first($modes)) {
                'all' => [$this->rows($user)->pluck('id'), 'everything'],
                'dates' => $this->byDates($user),
                'last' => $this->lastPrompts($user),
            };
        } catch (\InvalidArgumentException $e) {
            $this->error($e->getMessage());

            return self::FAILURE;
        }

        $prompts = $this->rows($user)->whereIn('id', $ids)->humanPrompts()->count();
        $this->line("{$user->name} <{$user->email}>: {$what}");
        $this->line("  {$ids->count()} interaction(s), of which {$prompts} typed prompt(s), plus their stored text.");

        if ($ids->isEmpty() || $this->option('dry-run')) {
            $this->line($ids->isEmpty() ? '  Nothing to delete.' : '  Dry run: nothing deleted.');

            return self::SUCCESS;
        }

        if (! $this->option('force') && ! $this->confirm('Delete this permanently? It cannot be undone.')) {
            $this->line('Nothing deleted.');

            return self::SUCCESS;
        }

        $sessions = collect();
        foreach ($ids->chunk(200) as $chunk) {
            $rows = $this->rows($user)->whereIn('id', $chunk)->get(['id', 'ai_session_id', 'prompt_object', 'answer_object']);
            foreach ($rows as $row) {
                $bodies->delete($row->prompt_object);
                $bodies->delete($row->answer_object);
            }
            $sessions = $sessions->merge($rows->pluck('ai_session_id')->filter());
            // Scores go with the row (cascade); audit rows keep their history
            // with the interaction link cleared (null on delete).
            $this->rows($user)->whereIn('id', $chunk)->delete();
        }

        $this->tidySessions($user, $sessions->unique());
        $this->info("Deleted {$ids->count()} interaction(s).");

        return self::SUCCESS;
    }

    /** This person's rows, across the tenant scope (a command has no current tenant). */
    private function rows(User $user): Builder
    {
        return AiInteraction::withoutGlobalScope('tenant')
            ->where('tenant_id', $user->tenant_id)
            ->where('user_id', $user->id);
    }

    /** @return array{0: Collection, 1: string} */
    private function byDates(User $user): array
    {
        $from = $this->date('from')?->startOfDay();
        $to = $this->date('to')?->endOfDay();
        if ($from && $to && $from->gt($to)) {
            throw new \InvalidArgumentException('--from is after --to.');
        }

        $ids = $this->rows($user)
            ->when($from, fn ($q) => $q->where('occurred_at', '>=', $from))
            ->when($to, fn ($q) => $q->where('occurred_at', '<=', $to))
            ->pluck('id');

        return [$ids, sprintf('from %s to %s (%s)', $from?->toDateString() ?? 'the start', $to?->toDateString() ?? 'today', config('app.timezone'))];
    }

    private function date(string $option): ?Carbon
    {
        $value = $this->option($option);
        if (! $value) {
            return null;
        }
        if (! preg_match('/^\d{4}-\d{2}-\d{2}$/', $value) || ! strtotime($value)) {
            throw new \InvalidArgumentException("--{$option} must be a date like 2026-09-28.");
        }

        return Carbon::parse($value);
    }

    /**
     * The last N prompts and everything in their turns: from each prompt up to
     * (not including) the next prompt in the same session.
     *
     * @return array{0: Collection, 1: string}
     */
    private function lastPrompts(User $user): array
    {
        $n = filter_var($this->option('last'), FILTER_VALIDATE_INT, ['options' => ['min_range' => 1]]);
        if ($n === false) {
            throw new \InvalidArgumentException('--last must be a whole number, 1 or more.');
        }

        $prompts = $this->rows($user)->humanPrompts()
            ->orderByDesc('occurred_at')->orderByDesc('id')
            ->limit($n)
            ->get(['id', 'ai_session_id', 'occurred_at']);

        $ids = collect();
        foreach ($prompts as $prompt) {
            if (! $prompt->ai_session_id) {
                $ids->push($prompt->id);

                continue;
            }

            $inSession = fn () => $this->rows($user)->where('ai_session_id', $prompt->ai_session_id);
            $after = fn ($q) => $q->where('occurred_at', '>', $prompt->occurred_at)
                ->orWhere(fn ($q) => $q->where('occurred_at', $prompt->occurred_at)->where('id', '>', $prompt->id));

            $next = $inSession()->humanPrompts()->where($after)
                ->orderBy('occurred_at')->orderBy('id')->first(['id', 'occurred_at']);

            $turn = $inSession()->where(fn ($q) => $q->where('id', $prompt->id)->orWhere($after));
            if ($next) {
                $turn->where(fn ($q) => $q->where('occurred_at', '<', $next->occurred_at)
                    ->orWhere(fn ($q) => $q->where('occurred_at', $next->occurred_at)->where('id', '<', $next->id)));
            }
            $ids = $ids->merge($turn->pluck('id'));
        }

        return [$ids->unique()->values(), "the last {$prompts->count()} prompt(s) and their answers"];
    }

    /** Sessions that lost rows: drop the empty ones, re-measure the rest. */
    private function tidySessions(User $user, Collection $sessionIds): void
    {
        foreach ($sessionIds as $id) {
            $left = AiInteraction::withoutGlobalScope('tenant')->where('ai_session_id', $id);
            $count = (clone $left)->count();
            $session = AiSession::withoutGlobalScope('tenant')->where('tenant_id', $user->tenant_id)->find($id);

            if (! $session) {
                continue;
            }
            if ($count === 0) {
                $session->delete();

                continue;
            }
            $session->forceFill([
                'interaction_count' => $count,
                'started_at' => (clone $left)->min('occurred_at'),
                'ended_at' => (clone $left)->max('occurred_at'),
            ])->save();
        }
    }
}
