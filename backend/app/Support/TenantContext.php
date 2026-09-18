<?php

namespace App\Support;

/**
 * Holds the tenant the current request belongs to.
 *
 * Every query in this module is filtered by it automatically (see the
 * BelongsToTenant trait), so nothing depends on a developer remembering to add a
 * `where`. It is set by the ingestion middleware from the device token, and by the
 * auth layer for a logged-in user.
 */
class TenantContext
{
    private ?int $tenantId = null;

    public function set(?int $tenantId): void
    {
        $this->tenantId = $tenantId;
    }

    public function id(): ?int
    {
        return $this->tenantId;
    }

    public function has(): bool
    {
        return $this->tenantId !== null;
    }

    /**
     * Run a callback as a given tenant and restore the previous one afterwards.
     * Used by queued jobs, which have no request to take the tenant from.
     */
    public function runAs(int $tenantId, callable $callback): mixed
    {
        $previous = $this->tenantId;
        $this->tenantId = $tenantId;

        try {
            return $callback();
        } finally {
            $this->tenantId = $previous;
        }
    }
}
