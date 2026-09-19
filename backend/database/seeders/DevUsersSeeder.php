<?php

namespace Database\Seeders;

use App\Models\Tenant;
use App\Models\User;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\Hash;
use Illuminate\Support\Str;

/**
 * The three people used to look at the dashboard in development: a member, a
 * manager, and an admin who may read raw prompt text.
 *
 * They existed before this file did — typed in by hand during Phase 7 — all three
 * with the password "password". That is how a demo account reaches production:
 * not by anyone deciding it should, but by nobody being able to say where it came
 * from. Now there is one place that makes them, it refuses to run anywhere but
 * local and testing, and the password is never a word anyone can guess.
 *
 *   php artisan db:seed --class=DevUsersSeeder
 *   AIUL_SEED_PASSWORD=whatever php artisan db:seed --class=DevUsersSeeder
 *
 * Without AIUL_SEED_PASSWORD a random one is generated and printed once. Running
 * it again rotates the passwords, so an old one is never left working.
 */
class DevUsersSeeder extends Seeder
{
    public function run(): void
    {
        if (! app()->environment(['local', 'testing'])) {
            $this->command->error('DevUsersSeeder is for local development only. Refusing to run in '.app()->environment().'.');

            return;
        }

        $password = env('AIUL_SEED_PASSWORD') ?: Str::password(20);

        $tenant = Tenant::firstOrCreate(
            ['slug' => 'dev'],
            ['name' => 'Development', 'retention_days' => 90],
        );

        $people = [
            ['dev@example.com', 'Dev Member', 'member', false],
            ['manager@example.com', 'Meera Manager', 'manager', false],
            // The only one that may read what someone actually typed, and every
            // read of it writes a consent_records row naming who looked and why.
            ['admin@example.com', 'Arun Admin', 'admin', true],
        ];

        foreach ($people as [$email, $name, $role, $canViewRaw]) {
            User::withoutGlobalScope('tenant')->updateOrCreate(
                ['email' => $email],
                [
                    'tenant_id' => $tenant->id,
                    'name' => $name,
                    'role' => $role,
                    'can_view_raw_prompts' => $canViewRaw,
                    'password' => Hash::make($password),
                    'email_verified_at' => now(),
                ],
            );
        }

        $this->command->info('Seeded 3 development users in tenant "dev".');

        if (! env('AIUL_SEED_PASSWORD')) {
            $this->command->warn('Password for all three (shown once, not stored anywhere):');
            $this->command->line('    '.$password);
        }
    }
}
