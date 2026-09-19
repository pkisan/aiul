<?php

namespace App\Services;

use App\Models\Tenant;
use Illuminate\Contracts\Filesystem\Filesystem;
use Illuminate\Encryption\Encrypter;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Storage;
use Illuminate\Support\Str;

/**
 * Stores prompt and answer text in object storage, encrypted with a key belonging
 * to that one customer (D9).
 *
 * Two reasons this is not a database column:
 *   - bodies are large and rarely read, so they would bloat the table every
 *     dashboard query touches
 *   - retention can delete a body and keep the metrics, which is exactly what
 *     "purge raw content after 90 days, keep the scores" means
 *
 * ## Envelope encryption, in two sentences
 *
 * Each tenant has a random 32-byte data key. The bodies are encrypted with it, and
 * the data key itself is stored encrypted ("wrapped") on the tenants row, so the
 * database never holds a usable key. One customer's compromised key exposes that
 * customer and nobody else, which is the whole point of the change.
 *
 * Wrapping is the only part that differs in production: locally the wrapper is the
 * application key, and in production it is a KMS call. That is two methods,
 * `wrap` and `unwrap`, and nothing else in the class knows the difference.
 *
 * ## Bodies written before this existed
 *
 * They were encrypted with the application key directly. Stored ciphertext now
 * starts with a version marker, so a body says for itself which key it needs: the
 * old ones stay readable until retention deletes them, and no re-encryption pass
 * has to run over object storage.
 */
class BodyStore
{
    /**
     * Marks a body encrypted with its tenant's own data key. Bodies without this
     * prefix predate D9 and are encrypted with the application key.
     */
    private const TENANT_KEY_PREFIX = 'AIULv2:';

    /**
     * Unwrapped data keys, for this process only. A body write and the scoring job
     * that follows it both need the key, and unwrapping is a KMS call in
     * production, which is worth doing once. Never cached beyond the process, and
     * never written anywhere.
     *
     * @var array<int, string>
     */
    private array $keys = [];

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
        // is a prefix operation — and so a read can tell whose key it needs.
        $key = sprintf('%d/%s/%s-%s.enc', $tenantId, now()->format('Y/m/d'), $eventId, $kind);

        $this->disk()->put($key, self::TENANT_KEY_PREFIX.$this->cipher($tenantId)->encryptString($text));

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

        $stored = $this->disk()->get($key);

        if (! str_starts_with($stored, self::TENANT_KEY_PREFIX)) {
            // Written before D9: the application key is the only key it ever had.
            return Crypt::decryptString($stored);
        }

        return $this->cipher($this->tenantOf($key))
            ->decryptString(substr($stored, strlen(self::TENANT_KEY_PREFIX)));
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
        unset($this->keys[$tenantId]);
    }

    public static function newEventId(): string
    {
        return (string) Str::uuid();
    }

    /** The object key starts with the tenant id, which is how a read finds its key. */
    private function tenantOf(string $objectKey): int
    {
        return (int) Str::before($objectKey, '/');
    }

    /** An encrypter holding one tenant's data key. */
    private function cipher(int $tenantId): Encrypter
    {
        return new Encrypter($this->dataKey($tenantId), 'aes-256-gcm');
    }

    /**
     * One tenant's raw data key, created on first use.
     *
     * The creation is done under a row lock, and only when the column is still
     * null. Two workers writing a body for a new tenant at the same moment would
     * otherwise each generate a key, and whichever wrote second would make the
     * other's ciphertext unreadable forever.
     */
    private function dataKey(int $tenantId): string
    {
        if (isset($this->keys[$tenantId])) {
            return $this->keys[$tenantId];
        }

        $wrapped = DB::transaction(function () use ($tenantId) {
            $tenant = Tenant::query()->lockForUpdate()->findOrFail($tenantId);

            if (filled($tenant->data_key)) {
                return $tenant->data_key;
            }

            $fresh = $this->wrap(random_bytes(32));
            $tenant->forceFill(['data_key' => $fresh])->save();

            return $fresh;
        });

        return $this->keys[$tenantId] = $this->unwrap($wrapped);
    }

    /**
     * Wrap a raw data key for storage. THIS IS THE KMS SEAM: in production this
     * becomes an Encrypt call against the tenant's KMS key and returns the
     * ciphertext blob. Nothing else in the class changes.
     */
    private function wrap(string $rawKey): string
    {
        return Crypt::encryptString(base64_encode($rawKey));
    }

    /** The other half of the seam: a KMS Decrypt call in production. */
    private function unwrap(string $wrapped): string
    {
        return base64_decode(Crypt::decryptString($wrapped));
    }
}
