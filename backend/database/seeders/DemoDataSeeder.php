<?php

namespace Database\Seeders;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\Device;
use App\Models\Tenant;
use App\Models\User;
use App\Services\BodyStore;
use App\Support\TenantContext;
use Illuminate\Database\Seeder;
use Illuminate\Support\Arr;
use Illuminate\Support\Carbon;
use Illuminate\Support\Str;
use RuntimeException;

/**
 * Made-up usage for showing the dashboard: a "Demo Co" tenant with nine people,
 * five projects and 45 days of AI work, in its own tenant so it never mixes
 * with real captures.
 *
 *   php artisan db:seed --class=DemoDataSeeder
 *
 * Running it again deletes the demo tenant (everything under it cascades) and
 * builds it afresh. It refuses to run in production: nothing here is real.
 * Sign in as demo@example.com with the password it prints.
 */
class DemoDataSeeder extends Seeder
{
    private const PROJECTS = [
        '/Users/dev/pm-core' => ['Add a Kanban swimlane per assignee', 'Why does the task reorder endpoint return 422?', 'Write a migration for task dependencies', 'Refactor TaskPolicy so guests can comment'],
        '/Users/dev/client-portal' => ['Build an invoice PDF with the client logo', 'Make the login page pass WCAG AA contrast', 'Explain this Stripe webhook signature error'],
        '/Users/dev/plrb-lms' => ['Add SSO with WordPress to the course player', 'Fix the flaky quiz timer test', 'Cache the course catalogue query'],
        '/Users/dev/acme-wp-theme' => ['Convert this header to a block pattern', 'Why is my custom post type not in the REST API?', 'Speed up the product archive template'],
        '/Users/dev/mobile-app' => ['Offline sync for task comments in React Native', 'Push notification opens the wrong screen'],
        '' => ['Summarise this client email in three bullets', 'Draft a sprint retro agenda', 'Compare Postgres and MySQL JSON columns', 'Write a polite reminder for an overdue invoice'],
    ];

    private const TOOLS = [
        'claude-code' => ['api.anthropic.com', 'claude-sonnet-5-5'],
        'cursor' => ['api2.cursor.sh', 'claude-sonnet-5-5'],
        'chatgpt-web' => ['chatgpt.com', 'gpt-5'],
        'claude-web' => ['claude.ai', 'claude-opus-5-5'],
        'copilot-vscode' => ['api.githubcopilot.com', 'gpt-5-mini'],
        'codex' => ['chatgpt.com', 'gpt-5-codex'],
    ];

    // name => [main tool, second tool, projects, prompts per active day, days since last seen]
    private const PEOPLE = [
        'Asha Patel' => ['claude-code', 'claude-web', ['/Users/dev/pm-core', '/Users/dev/client-portal'], 9, 0],
        'Ravi Kumar' => ['cursor', 'chatgpt-web', ['/Users/dev/pm-core', '/Users/dev/mobile-app'], 7, 0],
        'Meera Shah' => ['claude-code', 'chatgpt-web', ['/Users/dev/plrb-lms'], 6, 0],
        'Kabir Singh' => ['chatgpt-web', 'claude-web', ['', '/Users/dev/acme-wp-theme'], 5, 0],
        'Nisha Rao' => ['copilot-vscode', 'chatgpt-web', ['/Users/dev/acme-wp-theme', '/Users/dev/client-portal'], 4, 1],
        'Dev Mehta' => ['codex', 'claude-code', ['/Users/dev/mobile-app', '/Users/dev/pm-core'], 5, 0],
        'Priya Nair' => ['claude-web', 'chatgpt-web', ['', '/Users/dev/plrb-lms'], 3, 2],
        'Arjun Iyer' => ['cursor', 'claude-code', ['/Users/dev/client-portal'], 4, 5],   // quiet: device silent
        'Tara Joshi' => ['chatgpt-web', 'claude-web', [''], 0, 9],                       // enrolled, did nothing
    ];

    private const SECRETS = [['aws-access-key'], ['github-token'], ['email'], ['aiul-device-token'], ['private-key']];

    private BodyStore $bodies;

    private bool $withBodies = true;

    public function run(): void
    {
        if (app()->environment('production')) {
            throw new RuntimeException('DemoDataSeeder writes made-up data; it never runs in production.');
        }

        mt_srand(42); // the same demo every time
        app(TenantContext::class)->set(null);
        $this->bodies = app(BodyStore::class);

        Tenant::where('slug', 'demo')->delete(); // cascades to users, devices, sessions, interactions
        $tenant = Tenant::create(['name' => 'Demo Co', 'slug' => 'demo', 'retention_days' => 90]);

        $password = Str::password(16, symbols: false);
        $this->person($tenant, 'Demo Manager', 'demo@example.com', User::ROLE_ADMIN, $password);

        $count = 0;
        foreach (self::PEOPLE as $name => [$tool, $second, $projects, $perDay, $lastSeenDays]) {
            $user = $this->person($tenant, $name, Str::slug($name, '.').'@demo.example.com', User::ROLE_MEMBER, Str::password(20));
            $device = $this->device($tenant, $user, Str::slug(Str::before($name, ' ')).'-mbp');
            $count += $this->work($tenant, $device, $user, $tool, $second, $projects, $perDay, $lastSeenDays);
        }

        // A machine nobody has been linked to yet: its work is "Unassigned device".
        $spare = $this->device($tenant, null, 'meeting-room-imac');
        $count += $this->work($tenant, $spare, null, 'chatgpt-web', 'chatgpt-web', [''], 1, 1);

        $this->command->info("Demo Co ready: 9 people, {$count} interactions".($this->withBodies ? '' : ' (no prompt text: object storage unreachable)').'.');
        $this->command->warn('Sign in as demo@example.com with password (shown once):');
        $this->command->line('    '.$password);
    }

    private function person(Tenant $tenant, string $name, string $email, string $role, string $password): User
    {
        $user = User::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id, 'name' => $name, 'email' => $email, 'role' => $role,
            'can_view_raw_prompts' => $role === User::ROLE_ADMIN, 'password' => $password,
        ]);
        $user->forceFill(['email_verified_at' => now()])->save();
        ConsentRecord::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id, 'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);

        return $user;
    }

    private function device(Tenant $tenant, ?User $user, string $hostname): Device
    {
        [, $hash] = Device::issueToken();

        return Device::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id, 'user_id' => $user?->id, 'hostname' => $hostname,
            'platform' => 'darwin', 'token_hash' => $hash,
        ]);
    }

    /** 45 days of sessions for one person, busier in recent weeks. Returns the interaction count. */
    private function work(Tenant $tenant, Device $device, ?User $user, string $tool, string $second, array $projects, int $perDay, int $lastSeenDays): int
    {
        $count = 0;
        $last = null;

        for ($day = 45; $day >= $lastSeenDays; $day--) {
            $date = now()->subDays($day)->startOfDay();
            if ($date->isWeekend() && mt_rand(0, 4) > 0) {
                continue;
            }
            // Adoption grows: about half the volume 6 weeks ago, full volume now.
            $prompts = (int) round($perDay * (1 - $day / 90) * mt_rand(60, 140) / 100);

            while ($prompts > 0) {
                $size = min($prompts, mt_rand(2, 6));
                $prompts -= $size;
                $start = $date->copy()->addHours(mt_rand(4, 13))->addMinutes(mt_rand(0, 59));
                // Today's work is in the past, and the busiest people are "active now".
                if ($day === 0) {
                    $start = now()->subMinutes(mt_rand($lastSeenDays === 0 && $perDay >= 6 ? 20 : 90, 300));
                }
                $count += $this->session($tenant, $device, $user, mt_rand(0, 3) ? $tool : $second, $projects[array_rand($projects)], $start, $size);
                $last = max($last ?? $start, $start);
            }
        }

        $device->forceFill(['last_seen_at' => $last ?? now()->subDays($lastSeenDays)])->save();

        return $count;
    }

    private function session(Tenant $tenant, Device $device, ?User $user, string $tool, string $repo, Carbon $at, int $prompts): int
    {
        [$host, $model] = self::TOOLS[$tool];
        // Browser chats happen outside a checkout, whatever the person's project.
        $inRepo = in_array($tool, ['claude-code', 'cursor', 'copilot-vscode', 'codex'], true) && $repo !== '';
        $repo = $inRepo ? $repo : null;

        $session = AiSession::withoutGlobalScope('tenant')->create([
            'tenant_id' => $tenant->id, 'device_id' => $device->id, 'user_id' => $user?->id,
            'tool' => $tool, 'repo' => $repo, 'branch' => $repo ? Arr::random(['main', 'feature/'.Str::slug(self::PROJECTS[$repo][0]), 'fix/'.mt_rand(100, 999)]) : null,
            'started_at' => $at, 'ended_at' => $at, 'interaction_count' => 0,
        ]);

        $n = 0;
        $time = $at->copy();
        for ($p = 0; $p < $prompts; $p++) {
            $text = self::PROJECTS[$repo ?? ''][array_rand(self::PROJECTS[$repo ?? ''])];
            $this->interaction($session, $host, $model, 'human', $text, $time);
            $n++;
            // Agent tools take a few steps on their own for each prompt.
            if (in_array($tool, ['claude-code', 'cursor', 'codex'], true)) {
                for ($s = mt_rand(1, 4); $s > 0; $s--) {
                    $time->addSeconds(mt_rand(10, 60));
                    $this->interaction($session, $host, $model, 'agent', null, $time);
                    $n++;
                }
            }
            $time->addMinutes(mt_rand(2, 12));
        }

        $session->update(['ended_at' => min($time, now()), 'interaction_count' => $n]);

        return $n;
    }

    private function interaction(AiSession $session, string $host, string $model, string $kind, ?string $prompt, Carbon $at): void
    {
        $eventId = 'demo-'.Str::uuid();
        $answer = $prompt ? "Here is one way to approach it:\n\n1. Start with the smallest change.\n2. Add a test that fails first.\n3. Then refactor." : null;
        // About one prompt in 60 had a secret pasted in; redaction masked it.
        $redacted = $kind === 'human' && mt_rand(1, 60) === 1 ? self::SECRETS[array_rand(self::SECRETS)] : null;

        AiInteraction::withoutGlobalScope('tenant')->create([
            'tenant_id' => $session->tenant_id, 'device_id' => $session->device_id, 'user_id' => $session->user_id,
            'ai_session_id' => $session->id, 'event_id' => $eventId, 'host' => $host, 'path' => '/v1/messages',
            'tool' => $session->tool, 'model' => $model, 'kind' => $kind, 'automated' => $kind !== 'human',
            'repo' => $session->repo, 'branch' => $session->branch,
            'prompt_chars' => strlen($prompt ?? 'tool result'), 'answer_chars' => strlen($answer ?? 'tool call'),
            'prompt_tokens' => mt_rand(400, 9000), 'response_tokens' => mt_rand(80, 1500),
            'request_bytes' => mt_rand(2000, 40000), 'response_bytes' => mt_rand(500, 9000),
            'duration_ms' => mt_rand(800, 25000), 'streamed' => true,
            'redaction_rules_version' => 2, 'allowlist_version' => 4, 'redacted' => $redacted,
            'prompt_object' => $prompt ? $this->body($session->tenant_id, $eventId, 'prompt', $redacted ? "{$prompt}\n\nkey: [REDACTED:{$redacted[0]}]" : $prompt) : null,
            'answer_object' => $answer ? $this->body($session->tenant_id, $eventId, 'answer', $answer) : null,
            'occurred_at' => $at,
        ]);
    }

    /** Prompt text for previews; the numbers still work if object storage is down. */
    private function body(int $tenantId, string $eventId, string $part, string $text): ?string
    {
        if (! $this->withBodies) {
            return null;
        }
        try {
            return $this->bodies->put($tenantId, $eventId, $part, $text);
        } catch (\Throwable) {
            $this->withBodies = false;

            return null;
        }
    }
}
