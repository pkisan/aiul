<?php

use App\Http\Controllers\Api\DevicePairingController;
use App\Http\Controllers\Api\EventIngestionController;
use App\Http\Middleware\AuthenticateDevice;
use Illuminate\Support\Facades\Route;

/**
 * The agent's endpoints. Device tokens authenticate it; nothing else does.
 */
Route::middleware(AuthenticateDevice::class)->prefix('aiul')->group(function () {
    Route::post('/events', [EventIngestionController::class, 'store'])->name('aiul.events.store');

    // `aiul login`: get a code, then poll until a signed-in person enters it.
    Route::post('/pair', [DevicePairingController::class, 'start'])->name('aiul.pair.start');
    Route::get('/pair', [DevicePairingController::class, 'status'])->name('aiul.pair.status');
});
