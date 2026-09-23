<?php

namespace App\Http\Controllers;

use App\Models\ConsentRecord;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Response;

class ConsentController extends Controller
{
    public function show(Request $request): Response|RedirectResponse
    {
        if ($request->user()->hasConsented()) {
            return redirect()->intended(route('dashboard'));
        }

        return Inertia::render('Consent', [
            'version' => config('aiul.consent_version'),
            'points' => [
                'Prompts and answers you send to AI tools on a company device are captured, with the AI account you used.',
                'Secrets and obvious personal data are masked before anything is stored.',
                'Traffic to anything that is not an AI provider is never decrypted or logged.',
                'Managers see usage per person and per project. Reading your actual prompt text needs a separate permission, and every read is written to an audit log.',
            ],
        ]);
    }

    /** The acceptance itself, recorded with the version of the notice that was shown. */
    public function store(Request $request): RedirectResponse
    {
        $request->validate(['accept' => ['accepted']]);

        ConsentRecord::create([
            'tenant_id' => $request->user()->tenant_id,
            'user_id' => $request->user()->id,
            'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'),
            'granted_at' => now(),
            'ip' => $request->ip(),
        ]);

        return redirect()->intended(route('dashboard'));
    }
}
