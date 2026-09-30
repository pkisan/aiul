<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Support\Facades\DB;

class Project extends Model
{
    use BelongsToTenant;

    protected $table = 'pm_projects';

    protected $guarded = [];

    public function tasks(): HasMany
    {
        return $this->hasMany(Task::class);
    }

    public function sprints(): HasMany
    {
        return $this->hasMany(Sprint::class);
    }

    public function remotes(): HasMany
    {
        return $this->hasMany(ProjectRemote::class);
    }

    /**
     * The next task number. Counted from every key this project has ever
     * issued (pm_task_keys), not just the tasks in it now: a task that moved
     * away keeps its old key, so its number must never be handed out again.
     * Call inside a transaction with this project's row locked.
     */
    public function nextNumber(): int
    {
        $issued = TaskKey::withoutGlobalScope('tenant')
            ->where('tenant_id', $this->tenant_id)
            ->where('key', 'like', $this->key.'-%')
            ->max(DB::raw("cast(split_part(key, '-', 2) as integer)"));

        return max((int) $issued, (int) $this->tasks()->max('number')) + 1;
    }
}
