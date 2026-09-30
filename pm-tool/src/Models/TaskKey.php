<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;

/** One key a task has had ("AAY-4"). A task keeps its old keys when it moves project. */
class TaskKey extends Model
{
    use BelongsToTenant;

    protected $table = 'pm_task_keys';

    protected $guarded = [];
}
