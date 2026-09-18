<?php

namespace App\Providers;

use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    /**
     * Register any application services.
     */
    public function register(): void
    {
        // One tenant context per request, so the global scope and the queue jobs
        // are talking about the same thing.
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
        //
    }
}
