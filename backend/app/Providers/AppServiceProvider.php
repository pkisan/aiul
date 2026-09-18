<?php

namespace App\Providers;

use Illuminate\Support\Facades\Vite;
use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    /**
     * Register any application services.
     */
    public function register(): void
    {
        // One tenant context per request, so the global scope on every model and
        // the queue jobs are talking about the same tenant.
        //
        // NOTE: `breeze:install` overwrites this file. If tenant isolation tests
        // start failing after running a Laravel installer, look here first.
        $this->app->singleton(\App\Support\TenantContext::class);

        $this->app->bind(\App\Services\BodyStore::class, fn () => new \App\Services\BodyStore(
            config('aiul.body_disk', 's3')
        ));
    }

    /**
     * Bootstrap any application services.
     */
    public function boot(): void
    {
        Vite::prefetch(concurrency: 3);
    }
}
