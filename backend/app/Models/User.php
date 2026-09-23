<?php

namespace App\Models;

// use Illuminate\Contracts\Auth\MustVerifyEmail;
use Database\Factories\UserFactory;
use Illuminate\Database\Eloquent\Attributes\Fillable;
use Illuminate\Database\Eloquent\Attributes\Hidden;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Foundation\Auth\User as Authenticatable;
use Illuminate\Notifications\Notifiable;

#[Fillable(['tenant_id', 'name', 'email', 'password', 'role', 'can_view_raw_prompts'])]
#[Hidden(['password', 'remember_token'])]
class User extends Authenticatable
{
    /** @use HasFactory<UserFactory> */
    use HasFactory, Notifiable;

    /**
     * Roles decide what a person sees.
     *
     *   member  — their own data only
     *   manager — aggregates for their tenant
     *   admin   — aggregates, and may be granted raw prompt access
     */
    public const ROLE_MEMBER = 'member';

    public const ROLE_MANAGER = 'manager';

    public const ROLE_ADMIN = 'admin';

    /** Managers and admins see the aggregate dashboard. */
    public function isManager(): bool
    {
        return in_array($this->role, [self::ROLE_MANAGER, self::ROLE_ADMIN], true);
    }

    public function isAdmin(): bool
    {
        return $this->role === self::ROLE_ADMIN;
    }

    /**
     * Whether this person may read actual prompt text.
     *
     * Deliberately narrow: being an admin is not enough on its own, the flag has
     * to be granted as well, and every use of it is audit-logged where it is used.
     * That separation is the difference between coaching and surveillance.
     */
    public function canViewRawPrompts(): bool
    {
        return $this->isAdmin() && (bool) $this->can_view_raw_prompts;
    }

    /** Whether this person accepted the current capture notice and has not withdrawn. */
    public function hasConsented(): bool
    {
        return ConsentRecord::withoutGlobalScope('tenant')
            ->where('user_id', $this->id)
            ->where('kind', ConsentRecord::KIND_CAPTURE)
            ->where('policy_version', config('aiul.consent_version'))
            ->whereNotNull('granted_at')
            ->whereNull('revoked_at')
            ->exists();
    }

    /**
     * Get the attributes that should be cast.
     *
     * @return array<string, string>
     */
    protected function casts(): array
    {
        return [
            'email_verified_at' => 'datetime',
            'password' => 'hashed',
            'can_view_raw_prompts' => 'boolean',
        ];
    }
}
