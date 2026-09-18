<?php

namespace App\Http\Middleware;

use App\Support\TenantContext;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * Sets the tenant for a logged-in person, the same way the device middleware does
 * for an agent. From here on every model query is confined to their tenant.
 */
class SetTenantFromUser
{
    public function handle(Request $request, Closure $next): Response
    {
        if ($user = $request->user()) {
            app(TenantContext::class)->set($user->tenant_id);
        }

        return $next($request);
    }
}
