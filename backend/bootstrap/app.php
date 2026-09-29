<?php

use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Exceptions;
use Illuminate\Foundation\Configuration\Middleware;
use Illuminate\Http\Request;

return Application::configure(basePath: dirname(__DIR__))
    ->withRouting(
        web: __DIR__.'/../routes/web.php',
        api: __DIR__.'/../routes/api.php',
        commands: __DIR__.'/../routes/console.php',
        health: '/up',
    )
    ->withMiddleware(function (Middleware $middleware): void {
        // The tenant comes from the signed-in user, who is read from the session:
        // so AFTER StartSession (a prepend ran before it, found no user, set no
        // tenant, and every tenant's rows showed). And BEFORE SubstituteBindings,
        // or a row belonging to another tenant is loaded and then refused by the
        // policy, a 403 that admits the row exists; with the tenant set first,
        // the global scope means it is simply not found.
        $middleware->web(append: [
            \App\Http\Middleware\SetTenantFromUser::class,
        ]);
        $middleware->prependToPriorityList(
            before: \Illuminate\Routing\Middleware\SubstituteBindings::class,
            prepend: \App\Http\Middleware\SetTenantFromUser::class,
        );

        $middleware->web(append: [
            \App\Http\Middleware\HandleInertiaRequests::class,
            \Illuminate\Http\Middleware\AddLinkHeadersForPreloadedAssets::class,
            // Ends every other session of a person whose password changed, so
            // an admin's "Reset password" really locks a leaver out.
            \Illuminate\Session\Middleware\AuthenticateSession::class,
            \App\Http\Middleware\SecurityHeaders::class,
        ]);

        // Plesk puts nginx in front of Apache on the same host: trust it for
        // the client IP (audit log) and for "this request was HTTPS".
        $middleware->trustProxies(at: '127.0.0.1');
    })
    ->withExceptions(function (Exceptions $exceptions): void {
        $exceptions->shouldRenderJsonWhen(
            fn (Request $request) => $request->is('api/*') || $request->expectsJson(),
        );
    })->create();
