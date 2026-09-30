<?php

namespace Pm;

use Illuminate\Support\ServiceProvider;

/**
 * Plugs the PM module (everything under pm-tool/) into the backend app.
 * Registered in backend/bootstrap/providers.php; see D19 in docs/DECISIONS.md.
 */
class PmServiceProvider extends ServiceProvider
{
    public function boot(): void
    {
        $this->loadMigrationsFrom(__DIR__.'/../database/migrations');
    }
}
