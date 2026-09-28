<?php

namespace App\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class ConsentRecord extends Model
{
    use BelongsToTenant, HasFactory;

    public const KIND_CAPTURE = 'capture';

    public const KIND_RAW_VIEW = 'raw_view';

    /**
     * Someone opened a session and saw the first lines of its prompts and
     * answers. Recorded once per session view, not once per row: a page that
     * writes forty audit records teaches people to ignore the audit log.
     */
    public const KIND_SESSION_VIEW = 'session_view';

    /** Someone saw prompt previews in the usage page's list. Once per person per page. */
    public const KIND_LIST_VIEW = 'list_view';

    /** Every kind that means "someone saw prompt text": the audit log lists all of them. */
    public const READ_KINDS = [self::KIND_RAW_VIEW, self::KIND_SESSION_VIEW, self::KIND_LIST_VIEW];

    protected $guarded = [];

    /** Who looked. */
    public function actor(): BelongsTo
    {
        return $this->belongsTo(User::class, 'actor_user_id');
    }

    /** Whose words were looked at. */
    public function subject(): BelongsTo
    {
        return $this->belongsTo(User::class, 'user_id');
    }

    protected function casts(): array
    {
        return [
            'granted_at' => 'datetime',
            'revoked_at' => 'datetime',
        ];
    }
}
