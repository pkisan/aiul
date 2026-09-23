<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * Nobody uses the application before accepting the capture notice. The first
 * sign-in, and the first after the notice changes, lands on the consent page.
 */
class EnsureConsented
{
    public function handle(Request $request, Closure $next): Response
    {
        if ($request->user() && ! $request->user()->hasConsented()) {
            return redirect()->guest(route('consent.show'));
        }

        return $next($request);
    }
}
