<?php

namespace App\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class AiSession extends Model
{
    use BelongsToTenant, HasFactory;

    protected $guarded = [];

    protected function casts(): array
    {
        return [
            'started_at' => 'datetime',
            'ended_at' => 'datetime',
        ];
    }

    public function interactions(): HasMany
    {
        return $this->hasMany(AiInteraction::class);
    }

    /**
     * How long this session lasted. This is the definition behind "AI time per
     * task" on the dashboard, and the dashboard shows it, because a number nobody
     * can explain is a number nobody trusts.
     */
    public function durationSeconds(): int
    {
        if (! $this->ended_at) {
            return 0;
        }

        return max(0, $this->ended_at->diffInSeconds($this->started_at, absolute: true));
    }
}
