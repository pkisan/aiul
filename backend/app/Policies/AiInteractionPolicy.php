<?php

namespace App\Policies;

use App\Models\AiInteraction;
use App\Models\User;

class AiInteractionPolicy
{
    /** Anyone in the tenant may see that an interaction happened. */
    public function view(User $user, AiInteraction $interaction): bool
    {
        if ($user->tenant_id !== $interaction->tenant_id) {
            return false;
        }

        return $user->isManager() || $user->id === $interaction->user_id;
    }

    /**
     * Reading the actual prompt and answer text is a different question entirely.
     *
     * Two paths: it is your own prompt, or you hold the explicit raw-prompt
     * permission. A manager does NOT get this by being a manager.
     */
    public function viewRaw(User $user, AiInteraction $interaction): bool
    {
        if ($user->tenant_id !== $interaction->tenant_id) {
            return false;
        }

        if ($user->id === $interaction->user_id) {
            return true; // your own words
        }

        return $user->canViewRawPrompts();
    }
}
