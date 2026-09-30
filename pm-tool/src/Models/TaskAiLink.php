<?php

namespace Pm\Models;

use App\Models\AiSession;
use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/**
 * Which task an AI session was for, how we know (method) and how sure we are
 * (confidence, 0 to 1). A person confirming it makes it 1.0.
 */
class TaskAiLink extends Model
{
    use BelongsToTenant;

    public const EXPLICIT = 'explicit';       // active task at session start, 1.0

    public const CONVENTION = 'convention';   // task key in branch or prompt, 0.9

    public const TIME_WINDOW = 'time_window'; // only in-progress task, 0.6

    public const MANUAL = 'manual';           // set by a person in the inbox, 1.0

    protected $table = 'pm_task_ai_links';

    protected $guarded = [];

    protected function casts(): array
    {
        return ['confidence' => 'float', 'confirmed_at' => 'datetime'];
    }

    public function task(): BelongsTo
    {
        return $this->belongsTo(Task::class);
    }

    public function session(): BelongsTo
    {
        return $this->belongsTo(AiSession::class, 'ai_session_id');
    }
}
