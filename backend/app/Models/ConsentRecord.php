<?php

namespace App\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class ConsentRecord extends Model
{
    use BelongsToTenant, HasFactory;

    public const KIND_CAPTURE = 'capture';

    public const KIND_RAW_VIEW = 'raw_view';

    protected $guarded = [];

    protected function casts(): array
    {
        return [
            'granted_at' => 'datetime',
            'revoked_at' => 'datetime',
        ];
    }
}
