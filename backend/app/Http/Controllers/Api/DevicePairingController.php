<?php

namespace App\Http\Controllers\Api;

use App\Http\Controllers\Controller;
use App\Models\Device;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Str;

/**
 * The agent's half of `aiul login`: ask for a code, then ask whether someone has
 * entered it yet. The person's half is PairDeviceController on the web side.
 */
class DevicePairingController extends Controller
{
    /** A fresh code, valid for ten minutes. Asking again replaces the old one. */
    public function start(Request $request): JsonResponse
    {
        /** @var Device $device */
        $device = $request->attributes->get('device');

        // Letters and digits that cannot be misread for each other (no 0/O, 1/I).
        $alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
        $code = '';
        for ($i = 0; $i < 8; $i++) {
            $code .= $alphabet[random_int(0, strlen($alphabet) - 1)];
        }
        $code = substr($code, 0, 4).'-'.substr($code, 4);

        $device->forceFill([
            'pair_code' => $code,
            'pair_code_expires_at' => now()->addMinutes(10),
        ])->save();

        return response()->json([
            'code' => $code,
            'url' => route('pair.show'),
            'expires_at' => $device->pair_code_expires_at->toIso8601String(),
        ]);
    }

    /** Who this device belongs to now, and whether they have accepted the notice. */
    public function status(Request $request): JsonResponse
    {
        /** @var Device $device */
        $device = $request->attributes->get('device');
        $user = $device->user;

        return response()->json([
            'paired' => $user !== null,
            'name' => $user?->name,
            'email' => $user?->email,
            'consented' => (bool) $user?->hasConsented(),
            'code_pending' => $device->pair_code !== null && $device->pair_code_expires_at?->isFuture(),
        ]);
    }
}
