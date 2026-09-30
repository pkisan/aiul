<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;

/** One status or assignee change on a task. */
class TaskEvent extends Model
{
    use BelongsToTenant;

    public $timestamps = false;

    protected $table = 'pm_task_events';

    protected $guarded = [];

    protected function casts(): array
    {
        return ['occurred_at' => 'datetime'];
    }
}
