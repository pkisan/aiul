<?php

namespace App\Console\Commands;

use App\Models\Device;
use App\Models\Tenant;
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
                            {--platform=darwin}';

    protected $description = 'Register a device and print a one-time ingestion token';

    public function handle(): int
    {
        $slug = $this->option('tenant') ?: 'default';

        $tenant = Tenant::firstOrCreate(
            ['slug' => $slug],
            ['name' => str($slug)->headline()->toString()]
        );

        [$plain, $hash] = Device::issueToken();

        $device = Device::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id,
            'hostname' => $this->argument('hostname'),
            'platform' => $this->option('platform'),
            'token_hash' => $hash,
        ]);

        $this->info("Device #{$device->id} registered for tenant '{$tenant->slug}'.");
        $this->newLine();
        $this->line('  Token (shown once — copy it now):');
        $this->line("  {$plain}");
        $this->newLine();
        $this->line('  On the device, store it with:');
        $this->line("  security add-generic-password -U -s com.aiul.agent -a device-token -w '{$plain}'");

        return self::SUCCESS;
    }
}
