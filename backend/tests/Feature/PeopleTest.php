<?php

namespace Tests\Feature;

use App\Models\ConsentRecord;
use App\Models\Device;
use App\Models\Tenant;
use App\Models\User;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Hash;
use Tests\TestCase;

class PeopleTest extends TestCase
{
    use RefreshDatabase;

    private function user(string $role, ?Tenant $tenant = null): User
    {
        app(TenantContext::class)->set(null);
        $tenant ??= Tenant::firstOrCreate(['slug' => 'acme'], ['name' => 'Acme']);

        $user = User::create([
            'tenant_id' => $tenant->id, 'name' => ucfirst($role), 'email' => $role.'-'.uniqid().'@example.com',
            'password' => 'password', 'role' => $role,
        ]);
        ConsentRecord::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id, 'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);

        return $user;
    }

    public function test_only_admins_manage_people(): void
    {
        $this->actingAs($this->user(User::ROLE_MANAGER))->get('/people')->assertForbidden();
        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->post('/people', ['name' => 'X', 'email' => 'x@example.com', 'role' => 'admin'])->assertForbidden();
        $this->assertDatabaseMissing('users', ['email' => 'x@example.com']);
    }

    public function test_an_admin_adds_a_person_and_sees_the_password_once(): void
    {
        $admin = $this->user(User::ROLE_ADMIN);

        $this->actingAs($admin)
            ->post('/people', ['name' => 'Kim', 'email' => 'kim@example.com', 'role' => 'member', 'can_view_raw_prompts' => true])
            ->assertSessionHas('issued');

        $kim = User::where('email', 'kim@example.com')->first();
        $this->assertSame($admin->tenant_id, $kim->tenant_id);
        $this->assertFalse($kim->can_view_raw_prompts, 'prompt access is for admins only');
        $this->assertTrue(Hash::check(session('issued')['password'], $kim->password));
    }

    public function test_an_admin_cannot_touch_another_tenants_people_or_demote_themselves(): void
    {
        $admin = $this->user(User::ROLE_ADMIN);
        $stranger = $this->user(User::ROLE_MEMBER, Tenant::create(['slug' => 'other', 'name' => 'Other']));

        $this->actingAs($admin)->patch("/people/{$stranger->id}", ['role' => 'admin'])->assertNotFound();
        $this->actingAs($admin)->post("/people/{$stranger->id}/password")->assertNotFound();
        $this->actingAs($admin)->patch("/people/{$admin->id}", ['role' => 'member'])->assertStatus(422);
        $this->assertSame('admin', $admin->fresh()->role);
    }

    public function test_an_admin_issues_a_device_token_that_works_once_reissued(): void
    {
        $admin = $this->user(User::ROLE_ADMIN);
        $kim = $this->user(User::ROLE_MEMBER);

        $this->actingAs($admin)
            ->post("/people/{$kim->id}/devices", ['hostname' => 'DESKTOP-Q12UTEE', 'platform' => 'windows'])
            ->assertSessionHas('deviceToken');
        $first = session('deviceToken')['token'];

        $device = Device::byToken($first);
        $this->assertNotNull($device, 'the token shown must be the one that works');
        $this->assertSame($kim->id, $device->user_id);
        $this->assertSame('windows', $device->platform);

        // Same name again: one device, new token, old one dead.
        $this->actingAs($admin)->post("/people/{$kim->id}/devices", ['hostname' => 'DESKTOP-Q12UTEE', 'platform' => 'windows']);
        $this->assertNull(Device::byToken($first));
        $this->assertNotNull(Device::byToken(session('deviceToken')['token']));
        $this->assertSame(1, Device::withoutGlobalScope('tenant')->count());
    }

    public function test_renaming_a_device_keeps_its_token(): void
    {
        $admin = $this->user(User::ROLE_ADMIN);
        $kim = $this->user(User::ROLE_MEMBER);
        $this->actingAs($admin)->post("/people/{$kim->id}/devices", ['hostname' => 'old-mac', 'platform' => 'darwin']);
        $token = session('deviceToken')['token'];
        $this->actingAs($admin)->post("/people/{$kim->id}/devices", ['hostname' => 'taken', 'platform' => 'darwin']);
        $device = Device::byToken($token);

        $this->actingAs($admin)->patch("/people/{$kim->id}/devices/{$device->id}", ['hostname' => 'kims-macbook'])
            ->assertSessionHasNoErrors();
        $this->assertSame('kims-macbook', Device::byToken($token)?->hostname, 'same token, new name');

        $this->actingAs($admin)->patch("/people/{$kim->id}/devices/{$device->id}", ['hostname' => 'taken'])
            ->assertSessionHasErrors('hostname');
        $this->actingAs($admin)->patch("/people/{$kim->id}/devices/{$device->id}", ['hostname' => 'a b'])
            ->assertSessionHasErrors('hostname');
        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->patch("/people/{$kim->id}/devices/{$device->id}", ['hostname' => 'x'])->assertForbidden();
        // A device can only be renamed under the person it belongs to.
        $this->actingAs($admin)->patch("/people/{$admin->id}/devices/{$device->id}", ['hostname' => 'x'])->assertNotFound();
        $stranger = $this->user(User::ROLE_MEMBER, Tenant::create(['slug' => 'other', 'name' => 'Other']));
        $this->actingAs($admin)->patch("/people/{$stranger->id}/devices/{$device->id}", ['hostname' => 'x'])->assertNotFound();

        $this->assertSame('kims-macbook', Device::byToken($token)?->hostname);
    }

    public function test_device_tokens_are_admin_only_and_names_are_plain(): void
    {
        $kim = $this->user(User::ROLE_MEMBER);
        $stranger = $this->user(User::ROLE_MEMBER, Tenant::create(['slug' => 'other', 'name' => 'Other']));
        $admin = $this->user(User::ROLE_ADMIN);

        $this->actingAs($this->user(User::ROLE_MANAGER))
            ->post("/people/{$kim->id}/devices", ['hostname' => 'mac', 'platform' => 'darwin'])->assertForbidden();
        $this->actingAs($admin)
            ->post("/people/{$stranger->id}/devices", ['hostname' => 'mac', 'platform' => 'darwin'])->assertNotFound();
        $this->actingAs($admin)
            ->post("/people/{$kim->id}/devices", ['hostname' => 'mac; rm -rf /', 'platform' => 'darwin'])
            ->assertSessionHasErrors('hostname');
        $this->actingAs($admin)
            ->post("/people/{$kim->id}/devices", ['hostname' => 'mac', 'platform' => 'beos'])
            ->assertSessionHasErrors('platform');

        $this->assertSame(0, Device::withoutGlobalScope('tenant')->count());
    }

    public function test_the_seeder_makes_one_super_admin_with_a_generated_password(): void
    {
        putenv('AIUL_ADMIN_EMAIL=Owner@Example.com');
        try {
            $this->artisan('db:seed')->assertSuccessful();
        } finally {
            putenv('AIUL_ADMIN_EMAIL');
        }

        $this->assertSame(1, User::count());
        $owner = User::first();
        $this->assertSame('owner@example.com', $owner->email);
        $this->assertTrue($owner->canViewRawPrompts());
        $this->assertFalse(Hash::check('password', $owner->password));
    }
}
