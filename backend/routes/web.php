<?php

use App\Http\Controllers\ConsentController;
use App\Http\Controllers\DeletedPromptsController;
use App\Http\Controllers\PairDeviceController;
use App\Http\Controllers\PeopleController;
use App\Http\Controllers\ProfileController;
use App\Http\Middleware\EnsureConsented;
use Illuminate\Http\Request;
use App\Http\Controllers\UsageDashboardController;
use App\Http\Middleware\SetTenantFromUser;
use Illuminate\Support\Facades\Route;

// No landing page: the application IS the report. Managers open on /usage,
// everyone else on what was captured about them.
Route::get('/', function (Request $request) {
    if (! $request->user()) {
        return redirect()->route('login');
    }

    return redirect()->route($request->user()->isManager() ? 'usage.index' : 'usage.my-data');
})->name('home');

// The capture notice, accepted once per version before anything else is usable.
Route::middleware('auth')->group(function () {
    Route::get('/consent', [ConsentController::class, 'show'])->name('consent.show');
    Route::post('/consent', [ConsentController::class, 'store'])->name('consent.store');
});

Route::middleware('auth')->group(function () {
    Route::get('/profile', [ProfileController::class, 'edit'])->name('profile.edit');
    Route::patch('/profile', [ProfileController::class, 'update'])->name('profile.update');
});

require __DIR__.'/auth.php';


/*
 * The AI usage dashboard. SetTenantFromUser confines every query below to the
 * signed-in person's tenant.
 */
Route::middleware(['auth', 'verified', SetTenantFromUser::class, EnsureConsented::class])->group(function () {
    // Anyone: link the device `aiul login` is running on to my account.
    Route::get('/pair', [PairDeviceController::class, 'show'])->name('pair.show');
    Route::post('/pair', [PairDeviceController::class, 'store'])->name('pair.store');

    // Anyone: what was captured about me.
    // Admins: the accounts that can sign in (there is no public sign-up).
    Route::get('/people', [PeopleController::class, 'index'])->name('people.index');
    Route::post('/people', [PeopleController::class, 'store'])->name('people.store');
    Route::patch('/people/{person}', [PeopleController::class, 'update'])->name('people.update');
    Route::post('/people/{person}/password', [PeopleController::class, 'resetPassword'])->name('people.password');
    Route::post('/people/{person}/devices', [PeopleController::class, 'issueDevice'])->name('people.devices');
    Route::patch('/people/{person}/devices/{device}', [PeopleController::class, 'renameDevice'])
        ->whereNumber('device')->name('people.devices.rename');

    Route::get('/my-data', [UsageDashboardController::class, 'myData'])->name('usage.my-data');

    // Managers: the aggregate view.
    Route::get('/usage', [UsageDashboardController::class, 'index'])->name('usage.index');
    // Two segments, so these never collide with /usage/{interaction} below.
    Route::get('/usage/session/{session}', [UsageDashboardController::class, 'session'])->name('usage.session');
    Route::get('/usage/project', [UsageDashboardController::class, 'project'])->name('usage.project');
    // Admins: what was deleted from the dashboard, restorable until purged.
    Route::get('/usage/deleted', [DeletedPromptsController::class, 'index'])->name('usage.deleted');
    Route::post('/usage/deleted/{deletion}/restore', [DeletedPromptsController::class, 'restore'])->whereUuid('deletion')->name('usage.deleted.restore');
    Route::delete('/usage/deleted/{deletion}', [DeletedPromptsController::class, 'destroy'])->whereUuid('deletion')->name('usage.deleted.destroy');
    Route::get('/usage/{interaction}', [UsageDashboardController::class, 'show'])->name('usage.show');
    Route::delete('/usage/{interaction}', [UsageDashboardController::class, 'destroy'])->name('usage.destroy');

    // Old links to the separate text page.
    Route::get('/usage/{interaction}/raw', [UsageDashboardController::class, 'raw'])->name('usage.raw');
});
