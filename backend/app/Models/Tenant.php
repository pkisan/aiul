<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Tenant extends Model
{
    use HasFactory;

    protected $fillable = ['name', 'slug', 'retention_days'];

    public function devices(): HasMany
    {
        return $this->hasMany(Device::class);
    }

    public function interactions(): HasMany
    {
        return $this->hasMany(AiInteraction::class);
    }
}
