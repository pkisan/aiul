<?php

use App\Http\Middleware\EnsureConsented;
use Illuminate\Support\Facades\Route;
use Pm\Http\InboxController;
use Pm\Http\ProjectController;
use Pm\Http\TaskController;

// Loaded by PmServiceProvider. The web group already sets the tenant from the
// signed-in person, so every Pm model below is confined to their tenant.
Route::middleware(['web', 'auth', 'verified', EnsureConsented::class])->prefix('pm')->name('pm.')->group(function () {
    Route::get('/', [ProjectController::class, 'home'])->name('home');

    // Everyone sees projects; managers and admins create and change them.
    Route::get('/projects', [ProjectController::class, 'index'])->name('projects.index');
    Route::post('/projects', [ProjectController::class, 'store'])->name('projects.store');
    Route::patch('/projects/{project}', [ProjectController::class, 'update'])->name('projects.update');
    Route::post('/projects/{project}/sprints', [ProjectController::class, 'storeSprint'])->name('sprints.store');

    // Everyone in the tenant works on tasks.
    Route::get('/projects/{project}/board', [TaskController::class, 'board'])->name('board');
    Route::post('/projects/{project}/tasks', [TaskController::class, 'store'])->name('tasks.store');
    Route::get('/tasks/{task}', [TaskController::class, 'show'])->name('tasks.show');
    Route::patch('/tasks/{task}', [TaskController::class, 'update'])->name('tasks.update');
    Route::post('/tasks/{task}/start', [TaskController::class, 'start'])->name('tasks.start');
    Route::post('/work/stop', [TaskController::class, 'stop'])->name('work.stop');

    // The unlinked inbox (a drawer; JSON). My sessions, or a manager's team view.
    Route::get('/inbox', [InboxController::class, 'index'])->name('inbox.index');
    Route::post('/inbox/{session}', [InboxController::class, 'decide'])->name('inbox.decide');
});
