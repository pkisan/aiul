<?php

namespace App\Console\Commands;

use App\Models\Device;
use App\Models\Tenant;
use App\Models\User;
use Illuminate\Console\Command;

/**
 * Issues a device token. The plain token is shown ONCE and only its hash is kept,
 * so this output is the only chance to copy it into the machine's keychain.
 */
class ProvisionDevice extends Command
{
    protected $signature = 'aiul:provision-device
                            {hostname : the machine this token is for}
                            {--tenant= : the tenant slug (created if it does not exist)}
                            {--user= : the email of the person who uses this machine}
                            {--platform=darwin}';

    protected $description = 'Register a device and print a one-time ingestion token';

    public function handle(): int
    {
        $slug = $this->option('tenant') ?: 'default';

        $tenant = Tenant::firstOrCreate(
            ['slug' => $slug],
            ['name' => str($slug)->headline()->toString()]
        );

        // Who uses this machine. Without it every interaction from the device shows
        // as "Unassigned device" on the dashboard, which is accurate and useless.
        $user = null;
        if ($email = $this->option('user')) {
            $user = User::withoutGlobalScope('tenant')
                ->where('tenant_id', $tenant->id)
                ->where('email', $email)
                ->first();

            if (! $user) {
                $this->error("No user {$email} in tenant '{$tenant->slug}'.");

                return self::FAILURE;
            }
        }

        [$plain, $hash] = Device::issueToken();

        // One record per machine. Re-provisioning replaces the token rather than
        // adding a second device with the same hostname: running this twice used to
        // leave a trail of rows, and a device that reports under one of them looks
        // like a different machine from the one that reported under the other.
        $device = Device::withoutGlobalScope('tenant')->updateOrCreate(
            [
                'tenant_id' => $tenant->id,
                'hostname' => $this->argument('hostname'),
            ],
            [
                'platform' => $this->option('platform'),
                'token_hash' => $hash,
                // Keep an existing assignment when --user is not given.
                'user_id' => $user?->id ?? Device::withoutGlobalScope('tenant')
                    ->where('tenant_id', $tenant->id)
                    ->where('hostname', $this->argument('hostname'))
                    ->value('user_id'),
            ],
        );

        $this->info("Device #{$device->id} registered for tenant '{$tenant->slug}'.");
        $this->line($device->user_id
            ? "  Interactions will be attributed to: {$device->user->name} <{$device->user->email}>"
            : '  NOT assigned to a person — the dashboard will show "Unassigned device".');
        $this->line('  Any token issued for this hostname before now has stopped working.');
        $this->newLine();
        $this->line('  Token (shown once — copy it now):');
        $this->line("  {$plain}");
        $this->newLine();
        $this->line('  On the device, store it with:');
        $this->line("  security add-generic-password -U -s com.aiul.agent -a device-token -w '{$plain}'");

        return self::SUCCESS;
    }
}
