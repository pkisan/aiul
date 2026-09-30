<?php

namespace Pm;

use Illuminate\Console\Scheduling\Schedule;
use Illuminate\Support\ServiceProvider;
use Inertia\Inertia;
use Pm\Console\LinkCommand;
use Pm\Http\InboxController;
use Pm\Models\WorkPeriod;

/**
 * Plugs the PM module (everything under pm-tool/) into the backend app.
 * Registered in backend/bootstrap/providers.php; see D19 in docs/DECISIONS.md.
 */
class PmServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        // Where Inertia looks for page files when it checks they exist (tests).
        $pages = realpath(__DIR__.'/../resources/js/Pages');
        if ($pages) {
            $this->app['config']->push('inertia.page_paths', $pages);
            $this->app['config']->push('inertia.testing.page_paths', $pages);
        }
    }

    public function boot(): void
    {
        $this->loadMigrationsFrom(__DIR__.'/../database/migrations');
        $this->loadRoutesFrom(__DIR__.'/../routes/web.php');

        $this->commands([LinkCommand::class]);
        // Link sessions that got new prompts; PM changes relink at once anyway.
        $this->callAfterResolving(Schedule::class, fn (Schedule $s) => $s->command('pm:link')->everyFiveMinutes()->withoutOverlapping()->onOneServer());

        // How many of my AI sessions wait in the inbox (nav badge).
        Inertia::share('pmInboxCount', fn () => auth()->user() ? InboxController::countFor(auth()->user()) : 0);

        // The task I am working on, shown in the top bar on every page.
        Inertia::share('pmActiveTask', function () {
            $period = auth()->user()
                ? WorkPeriod::with('task.project')->where('user_id', auth()->id())->whereNull('ended_at')->first()
                : null;

            return $period ? [
                'id' => $period->task->id,
                'key' => $period->task->key,
                'title' => $period->task->title,
                'since' => $period->started_at,
            ] : null;
        });
    }
}
