<?php

namespace Tests\Feature;

use App\Models\ConsentRecord;
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
