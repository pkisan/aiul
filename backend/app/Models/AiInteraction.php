<?php

namespace App\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\SoftDeletes;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class AiInteraction extends Model
{
    use BelongsToTenant, HasFactory, SoftDeletes;

    protected $guarded = [];

    protected function casts(): array
    {
        return [
            'occurred_at' => 'datetime',
            'streamed' => 'boolean',
            'automated' => 'boolean',
            'redacted' => 'array',
        ];
    }

    public function session(): BelongsTo
    {
        return $this->belongsTo(AiSession::class, 'ai_session_id');
    }

    public function device(): BelongsTo
    {
        return $this->belongsTo(Device::class);
    }

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    /**
     * What a person typed, as opposed to an agent's own steps or the tool's
     * housekeeping. Rows from before `kind` existed fall back to `automated`.
     */
    public function scopeHumanPrompts(Builder $query): Builder
    {
        return $query->where(fn ($q) => $q->where('kind', 'human')
            ->orWhere(fn ($q) => $q->whereNull('kind')->where('automated', false)));
    }
}
