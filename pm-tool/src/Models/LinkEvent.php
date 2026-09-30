<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;

/**
 * One change to a session's task link. Append-only: never updated or deleted
 * by the app. actor_id null means the linker made the change.
 */
class LinkEvent extends Model
{
    use BelongsToTenant;

    public $timestamps = false;

    protected $table = 'pm_link_events';

    protected $guarded = [];

    protected function casts(): array
    {
        return ['confidence' => 'float', 'occurred_at' => 'datetime'];
    }

    /** Record one change. tenant_id is passed in: the linker also runs where no tenant is set. */
    public static function record(int $tenantId, int $sessionId, ?int $from, ?int $to, string $action, ?string $method = null, ?float $confidence = null, ?int $actorId = null): self
    {
        return static::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenantId, 'ai_session_id' => $sessionId,
            'from_task_id' => $from, 'to_task_id' => $to, 'action' => $action,
            'method' => $method, 'confidence' => $confidence, 'actor_id' => $actorId,
            'occurred_at' => now(),
        ]);
    }
}
