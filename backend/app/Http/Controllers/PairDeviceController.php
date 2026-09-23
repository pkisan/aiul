<?php

namespace App\Http\Controllers;

use App\Models\Device;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Response;

/**
 * The person's half of `aiul login`: type the code the device printed, and the
 * device is attributed to you from then on. Reached only after sign-in and
 * consent, so a paired device always has a person who accepted the notice.
 */
class PairDeviceController extends Controller
{
    public function show(Request $request): Response
    {
        return Inertia::render('Pair', [
            'devices' => Device::where('user_id', $request->user()->id)
                ->get(['id', 'hostname', 'platform', 'last_seen_at']),
            'status' => session('status'),
        ]);
    }

    public function store(Request $request): RedirectResponse
    {
        $code = strtoupper(trim((string) $request->input('code')));
        // Accept the code with or without its dash.
        if (strlen($code) === 8) {
            $code = substr($code, 0, 4).'-'.substr($code, 4);
        }

        // Tenant-scoped: a code from another company's device is simply not found.
        $device = Device::where('pair_code', $code)
            ->where('pair_code_expires_at', '>', now())
            ->first();

        if (! $device) {
            return back()->withErrors(['code' => 'That code is wrong or has expired. Run aiul login again for a new one.']);
        }

        $device->forceFill([
            'user_id' => $request->user()->id,
            'pair_code' => null,
            'pair_code_expires_at' => null,
        ])->save();

        return back()->with('status', "{$device->hostname} is now linked to your account.");
    }
}
