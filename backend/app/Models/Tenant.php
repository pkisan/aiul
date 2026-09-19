<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Tenant extends Model
{
    use HasFactory;

    protected $fillable = ['name', 'slug', 'retention_days'];

    /**
     * The wrapped data key is deliberately not fillable and never serialised: it
     * is written by BodyStore alone and has no business reaching a JSON response
     * or an Inertia prop.
     */
    protected $hidden = ['data_key'];

    public function devices(): HasMany
    {
        return $this->hasMany(Device::class);
    }

    public function interactions(): HasMany
    {
        return $this->hasMany(AiInteraction::class);
    }
}
