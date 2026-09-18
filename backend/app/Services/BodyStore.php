<?php

namespace App\Services;

use Illuminate\Contracts\Filesystem\Filesystem;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Support\Facades\Storage;
use Illuminate\Support\Str;

/**
 * Stores prompt and answer text in object storage, encrypted.
 *
 * Two reasons this is not a database column:
 *   - bodies are large and rarely read, so they would bloat the table every
 *     dashboard query touches
 *   - retention can delete a body and keep the metrics, which is exactly what
 *     "purge raw content after 90 days, keep the scores" means
 *
 * Encryption is Laravel's application key for now. The production shape is a
 * per-tenant key, noted in docs/DECISIONS.md — the interface here does not change
 * when that arrives.
 */
class BodyStore
{
    public function __construct(private readonly string $disk = 's3') {}

    private function disk(): Filesystem
    {
        return Storage::disk($this->disk);
    }

    /**
     * Write one body and return its object key, or null when there is nothing to
     * store. Text arrives already redacted — this class never sees a raw secret.
     */
    public function put(int $tenantId, string $eventId, string $kind, ?string $text): ?string
    {
        if (blank($text)) {
            return null;
        }

        // Tenant first in the path, so a per-tenant lifecycle rule or a deletion
        // is a prefix operation.
        $key = sprintf('%d/%s/%s-%s.enc', $tenantId, now()->format('Y/m/d'), $eventId, $kind);

        $this->disk()->put($key, Crypt::encryptString($text));

        return $key;
    }

    /**
     * Read a body back. Returns null if it has been purged by retention, which is
     * a normal state and not an error.
     */
    public function get(?string $key): ?string
    {
        if (blank($key) || ! $this->disk()->exists($key)) {
            return null;
        }

        return Crypt::decryptString($this->disk()->get($key));
    }

    public function delete(?string $key): void
    {
        if (filled($key)) {
            $this->disk()->delete($key);
        }
    }

    /** Delete everything belonging to one tenant, for offboarding. */
    public function deleteTenant(int $tenantId): void
    {
        $this->disk()->deleteDirectory((string) $tenantId);
    }

    public static function newEventId(): string
    {
        return (string) Str::uuid();
    }
}
