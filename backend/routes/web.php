<?php

use App\Http\Controllers\ProfileController;
use Illuminate\Foundation\Application;
use App\Http\Controllers\UsageDashboardController;
use App\Http\Middleware\SetTenantFromUser;
use Illuminate\Support\Facades\Route;
use Inertia\Inertia;

Route::get('/', function () {
    return Inertia::render('Welcome', [
        'canLogin' => Route::has('login'),
        'canRegister' => Route::has('register'),
        'laravelVersion' => Application::VERSION,
        'phpVersion' => PHP_VERSION,
    ]);
});

Route::get('/dashboard', function () {
    return Inertia::render('Dashboard');
})->middleware(['auth', 'verified'])->name('dashboard');

Route::middleware('auth')->group(function () {
    Route::get('/profile', [ProfileController::class, 'edit'])->name('profile.edit');
    Route::patch('/profile', [ProfileController::class, 'update'])->name('profile.update');
    Route::delete('/profile', [ProfileController::class, 'destroy'])->name('profile.destroy');
});

require __DIR__.'/auth.php';


/*
 * The AI usage dashboard. SetTenantFromUser confines every query below to the
 * signed-in person's tenant.
 */
Route::middleware(['auth', 'verified', SetTenantFromUser::class])->group(function () {
    // Anyone: what was captured about me.
    Route::get('/my-data', [UsageDashboardController::class, 'myData'])->name('usage.my-data');

    // Managers: the aggregate view and the audit log.
    Route::get('/usage', [UsageDashboardController::class, 'index'])->name('usage.index');
    Route::get('/usage/audit', [UsageDashboardController::class, 'audit'])->name('usage.audit');
    // Two segments, so this never collides with /usage/{interaction} below.
    Route::get('/usage/task/{task}', [UsageDashboardController::class, 'task'])->name('usage.task');
    Route::get('/usage/{interaction}', [UsageDashboardController::class, 'show'])->name('usage.show');

    // Raw prompt text: policy-checked and audit-logged on every single view.
    Route::get('/usage/{interaction}/raw', [UsageDashboardController::class, 'raw'])->name('usage.raw');
});
