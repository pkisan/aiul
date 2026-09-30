<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;

class Sprint extends Model
{
    use BelongsToTenant;

    protected $table = 'pm_sprints';

    protected $guarded = [];

    protected function casts(): array
    {
        return ['start_date' => 'date', 'end_date' => 'date'];
    }
}
