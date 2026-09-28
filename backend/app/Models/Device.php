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

    public const PLATFORMS = ['darwin', 'linux', 'windows'];

    /**
     * Register a machine, or give an existing one a new token, and return the
     * plain token ONCE. Used by `aiul:provision-device` and the People page.
     *
     * One record per machine: provisioning the same hostname again replaces the
     * token (the old one stops working) instead of adding a second row.
     * A null $userId keeps whoever the device was already assigned to.
     *
     * @return array{0: self, 1: string}
     */
    public static function provision(int $tenantId, string $hostname, string $platform, ?int $userId = null): array
    {
        [$plain, $hash] = static::issueToken();

        $existing = static::withoutGlobalScope('tenant')
            ->where('tenant_id', $tenantId)
            ->where('hostname', $hostname)
            ->first();

        $device = static::withoutGlobalScope('tenant')->updateOrCreate(
            ['tenant_id' => $tenantId, 'hostname' => $hostname],
            [
                'platform' => $platform,
                'token_hash' => $hash,
                'revoked' => false,
                'user_id' => $userId ?? $existing?->user_id,
            ],
        );

        return [$device, $plain];
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
