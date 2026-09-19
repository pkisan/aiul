<?php

namespace Tests\Feature;

use App\Models\Tenant;
use App\Services\BodyStore;
use App\Support\TenantContext;
use Illuminate\Contracts\Encryption\DecryptException;
use Illuminate\Encryption\Encrypter;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Support\Facades\Storage;
use Tests\TestCase;

/**
 * Per-tenant encryption (D9). The claim being tested is narrow and worth stating:
 * one customer's key opens that customer's bodies and nothing else, and the
 * database never holds a key that can be used as it stands.
 */
class BodyEncryptionTest extends TestCase
{
    use RefreshDatabase;

    protected function setUp(): void
    {
        parent::setUp();
        Storage::fake('s3');
        app(TenantContext::class)->set(null);
    }

    private function tenant(string $slug): Tenant
    {
        return Tenant::create(['name' => ucfirst($slug), 'slug' => $slug]);
    }

    public function test_a_body_round_trips(): void
    {
        $tenant = $this->tenant('acme');
        $bodies = app(BodyStore::class);

        $key = $bodies->put($tenant->id, BodyStore::newEventId(), 'prompt', 'refactor the login form');

        $this->assertSame('refactor the login form', $bodies->get($key));
    }

    public function test_each_tenant_gets_its_own_key_and_it_is_stored_wrapped(): void
    {
        $acme = $this->tenant('acme');
        $globex = $this->tenant('globex');
        $bodies = app(BodyStore::class);

        $bodies->put($acme->id, BodyStore::newEventId(), 'prompt', 'acme secret plan');
        $bodies->put($globex->id, BodyStore::newEventId(), 'prompt', 'globex secret plan');

        $acme->refresh();
        $globex->refresh();

        $this->assertNotNull($acme->data_key);
        $this->assertNotSame($acme->data_key, $globex->data_key);

        // What is in the column must not be a usable key: it decrypts to 32 raw
        // bytes only for someone holding the wrapping key.
        $this->assertNotSame(32, strlen($acme->data_key));
        $this->assertSame(32, strlen(base64_decode(Crypt::decryptString($acme->data_key))));
    }

    /**
     * The point of the whole change: stealing one tenant's data key must not open
     * another tenant's body.
     */
    public function test_one_tenants_key_cannot_read_another_tenants_body(): void
    {
        $acme = $this->tenant('acme');
        $globex = $this->tenant('globex');
        $bodies = app(BodyStore::class);

        $globexKey = $bodies->put($globex->id, BodyStore::newEventId(), 'prompt', 'globex secret plan');
        $bodies->put($acme->id, BodyStore::newEventId(), 'prompt', 'acme secret plan');

        $acmeRawKey = base64_decode(Crypt::decryptString($acme->refresh()->data_key));
        $stored = Storage::disk('s3')->get($globexKey);
        $ciphertext = substr($stored, strlen('AIULv2:'));

        $this->expectException(DecryptException::class);
        (new Encrypter($acmeRawKey, 'aes-256-gcm'))->decryptString($ciphertext);
    }

    public function test_the_stored_object_is_not_readable_as_plain_text(): void
    {
        $tenant = $this->tenant('acme');
        $bodies = app(BodyStore::class);

        $key = $bodies->put($tenant->id, BodyStore::newEventId(), 'prompt', 'a very distinctive sentence');

        $this->assertStringNotContainsString('distinctive', Storage::disk('s3')->get($key));
    }

    /**
     * Bodies written before D9 used the application key with no version marker.
     * They must keep working until retention removes them, because re-encrypting
     * object storage is not something to require of an upgrade.
     */
    public function test_a_body_written_before_per_tenant_keys_is_still_readable(): void
    {
        $tenant = $this->tenant('acme');
        $legacyKey = sprintf('%d/2026/01/01/legacy-prompt.enc', $tenant->id);

        Storage::disk('s3')->put($legacyKey, Crypt::encryptString('written under the old scheme'));

        $this->assertSame('written under the old scheme', app(BodyStore::class)->get($legacyKey));
        // And no key was invented for the tenant just by reading an old body.
        $this->assertNull($tenant->refresh()->data_key);
    }

    public function test_the_key_is_created_once_and_reused(): void
    {
        $tenant = $this->tenant('acme');
        $bodies = app(BodyStore::class);

        $first = $bodies->put($tenant->id, BodyStore::newEventId(), 'prompt', 'one');
        $keyAfterFirst = $tenant->refresh()->data_key;

        $second = $bodies->put($tenant->id, BodyStore::newEventId(), 'prompt', 'two');

        $this->assertSame($keyAfterFirst, $tenant->refresh()->data_key);
        $this->assertSame('one', $bodies->get($first));
        $this->assertSame('two', $bodies->get($second));
    }

    /** A second process must find the stored key rather than generating its own. */
    public function test_a_fresh_instance_reads_what_an_earlier_one_wrote(): void
    {
        $tenant = $this->tenant('acme');

        $key = (new BodyStore('s3'))->put($tenant->id, BodyStore::newEventId(), 'prompt', 'across processes');

        $this->assertSame('across processes', (new BodyStore('s3'))->get($key));
    }

    public function test_the_wrapped_key_is_never_serialised(): void
    {
        $tenant = $this->tenant('acme');
        app(BodyStore::class)->put($tenant->id, BodyStore::newEventId(), 'prompt', 'anything');

        $this->assertArrayNotHasKey('data_key', $tenant->refresh()->toArray());
    }
}
