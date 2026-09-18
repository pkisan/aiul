<?php

use App\Http\Controllers\Api\EventIngestionController;
use App\Http\Middleware\AuthenticateDevice;
use Illuminate\Support\Facades\Route;

/**
 * The agent's only endpoint. Device tokens authenticate it; nothing else does.
 */
Route::middleware(AuthenticateDevice::class)->prefix('aiul')->group(function () {
    Route::post('/events', [EventIngestionController::class, 'store'])->name('aiul.events.store');
});
