<?php

namespace App\Http\Middleware;

use App\Models\Device;
use App\Support\TenantContext;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * Authenticates an agent by its device token and establishes the tenant for the
 * rest of the request.
 *
 * Everything downstream relies on this: once the tenant is set, the global scope
 * on every model confines the request to that customer's data.
 */
class AuthenticateDevice
{
    public function handle(Request $request, Closure $next): Response
    {
        $token = $request->bearerToken();

        if (blank($token)) {
            return response()->json(['message' => 'A device token is required.'], 401);
        }

        $device = Device::byToken($token);

        if (! $device) {
            // The same message whether the token is unknown, malformed or revoked:
            // saying which would help someone guessing.
            return response()->json(['message' => 'Unknown or revoked device token.'], 401);
        }

        app(TenantContext::class)->set($device->tenant_id);
        $request->attributes->set('device', $device);

        $device->forceFill(['last_seen_at' => now()])->saveQuietly();

        return $next($request);
    }
}
