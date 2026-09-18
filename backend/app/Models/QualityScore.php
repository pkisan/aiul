<?php

namespace App\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class QualityScore extends Model
{
    use BelongsToTenant, HasFactory;

    protected $guarded = [];

    protected function casts(): array
    {
        return [
            'dimensions' => 'array',
            'reasons' => 'array',
        ];
    }

    public function interaction(): BelongsTo
    {
        return $this->belongsTo(AiInteraction::class, 'ai_interaction_id');
    }
}
