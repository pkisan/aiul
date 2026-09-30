<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/** A stretch of time a person said "I am working on this task". Open while ended_at is null. */
class WorkPeriod extends Model
{
    use BelongsToTenant;

    public $timestamps = false;

    protected $table = 'pm_work_periods';

    protected $guarded = [];

    protected function casts(): array
    {
        return ['started_at' => 'datetime', 'ended_at' => 'datetime'];
    }

    public function task(): BelongsTo
    {
        return $this->belongsTo(Task::class);
    }
}
