<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use App\Models\User;
use Illuminate\Database\Eloquent\Casts\Attribute;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Support\Facades\DB;

class Task extends Model
{
    use BelongsToTenant;

    /** Board columns, in order. */
    public const STATUSES = ['backlog', 'todo', 'in_progress', 'in_review', 'done'];

    protected $table = 'pm_tasks';

    protected $guarded = [];

    protected function casts(): array
    {
        return ['started_at' => 'datetime', 'completed_at' => 'datetime'];
    }

    /** A new task's key goes into pm_task_keys, where the linker looks keys up. */
    protected static function booted(): void
    {
        static::created(fn (Task $task) => TaskKey::withoutGlobalScope('tenant')->create([
            'tenant_id' => $task->tenant_id, 'task_id' => $task->id, 'key' => $task->key,
        ]));
    }

    /** Every key this task has had, the current one included. */
    public function keys(): HasMany
    {
        return $this->hasMany(TaskKey::class);
    }

    public function project(): BelongsTo
    {
        return $this->belongsTo(Project::class);
    }

    public function sprint(): BelongsTo
    {
        return $this->belongsTo(Sprint::class);
    }

    public function assignee(): BelongsTo
    {
        return $this->belongsTo(User::class, 'assignee_id');
    }

    public function events(): HasMany
    {
        return $this->hasMany(TaskEvent::class);
    }

    public function aiLinks(): HasMany
    {
        return $this->hasMany(TaskAiLink::class);
    }

    /** "AAY-42": the project key plus the task number. Needs `project` loaded. */
    protected function key(): Attribute
    {
        return Attribute::get(fn () => $this->project->key.'-'.$this->number);
    }

    /**
     * Change fields on the task and record who moved it where. Status and
     * assignee changes become TaskEvents (for cycle time); started_at is the
     * first move to in_progress, completed_at the last move to done.
     */
    public function applyChanges(array $changes, User $by): void
    {
        DB::transaction(function () use ($changes, $by) {
            $this->fill($changes);

            foreach (['status' => 'status', 'assignee_id' => 'assignee'] as $column => $field) {
                if ($this->isDirty($column)) {
                    $this->events()->create([
                        'user_id' => $by->id, 'field' => $field,
                        'from' => $this->getOriginal($column), 'to' => $this->getAttribute($column),
                        'occurred_at' => now(),
                    ]);
                }
            }

            if ($this->isDirty('status')) {
                if ($this->status === 'in_progress' && ! $this->started_at) {
                    $this->started_at = now();
                }
                $this->completed_at = $this->status === 'done' ? now() : null;
                if ($this->status === 'done') {
                    // Nobody is still "working on" a finished task.
                    WorkPeriod::where('task_id', $this->id)->whereNull('ended_at')->update(['ended_at' => now()]);
                }
            }

            $this->save();
        });
    }
}
