<?php

namespace App\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Support\Str;

class Device extends Model
{
    use BelongsToTenant, HasFactory;

    protected $fillable = ['tenant_id', 'user_id', 'hostname', 'platform', 'token_hash', 'revoked'];

    protected $hidden = ['token_hash', 'pair_code'];

    protected function casts(): array
    {
        return [
            'revoked' => 'boolean',
            'last_seen_at' => 'datetime',
            'pair_code_expires_at' => 'datetime',
        ];
    }

    /**
     * The person this machine belongs to. Null means nobody has been named, and
     * the dashboard then shows the interactions under "Unassigned device" rather
     * than guessing.
     */
    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    /**
     * Issue a token for a device. The plain token is returned ONCE, to be put in
     * the machine's keychain; only its hash is stored.
     */
    public static function issueToken(): array
    {
        $plain = 'aiul_'.Str::random(48);

        return [$plain, hash('sha256', $plain)];
    }

    /**
     * Find the device a presented token belongs to.
     *
     * The lookup is by hash, so the database never holds anything that could be
     * replayed, and a revoked device is treated as no device at all.
     */
    public static function byToken(string $plain): ?self
    {
        return static::withoutGlobalScope('tenant')
            ->where('token_hash', hash('sha256', $plain))
            ->where('revoked', false)
            ->first();
    }
}
