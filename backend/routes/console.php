<?php

use Illuminate\Foundation\Inspiring;
use Illuminate\Support\Facades\Artisan;
use Illuminate\Support\Facades\Schedule;

Artisan::command('inspire', function () {
    $this->comment(Inspiring::quote());
})->purpose('Display an inspiring quote');

/**
 * Retention runs nightly. The promise this module makes is that raw prompt text
 * does not live forever, and a promise nobody schedules is not a promise.
 */
Schedule::command('aiul:purge-bodies')->dailyAt('03:30')->onOneServer();
