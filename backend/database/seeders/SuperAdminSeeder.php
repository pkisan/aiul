<?php

namespace Database\Seeders;

use App\Models\Tenant;
use App\Models\User;
use Illuminate\Database\Seeder;
use Illuminate\Support\Str;

/**
 * The one account an installation starts with: a super admin who can read
 * prompt text and add everyone else from the People page.
 *
 *   AIUL_ADMIN_EMAIL=you@company.com php artisan db:seed --class=SuperAdminSeeder --force
 *
 * Without AIUL_ADMIN_EMAIL it asks. The password is generated, printed ONCE and
 * stored only as a hash. Running it again for the same email rotates the
 * password — that is also how a lost one is recovered.
 */
class SuperAdminSeeder extends Seeder
{
    public function run(): void
    {
        $email = strtolower(trim((string) (env('AIUL_ADMIN_EMAIL') ?: $this->command->ask('Super admin email'))));

        if (! filter_var($email, FILTER_VALIDATE_EMAIL)) {
            $this->command->error("Not an email address: '{$email}'. Nothing was changed.");

            return;
        }

        $slug = config('aiul.tenant');
        $tenant = Tenant::firstOrCreate(
            ['slug' => $slug],
            ['name' => env('AIUL_TENANT_NAME') ?: Str::headline($slug), 'retention_days' => 90],
        );

        $password = Str::password(20);

        $user = User::withoutGlobalScope('tenant')->updateOrCreate(
            ['email' => $email],
            [
                'tenant_id' => $tenant->id,
                'name' => env('AIUL_ADMIN_NAME') ?: 'Super Admin',
                'role' => User::ROLE_ADMIN,
                // Every read of prompt text is still written to the audit log.
                'can_view_raw_prompts' => true,
                'password' => $password, // hashed by the model's cast
            ],
        );
        $user->forceFill(['email_verified_at' => $user->email_verified_at ?? now()])->save();

        $this->command->info("Super admin {$email} ready in tenant '{$tenant->slug}'.");
        $this->command->warn('Password (shown once, not stored anywhere — copy it now):');
        $this->command->line('    '.$password);
    }
}
