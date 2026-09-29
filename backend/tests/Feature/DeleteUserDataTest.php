<?php

namespace Tests\Feature;

use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\ConsentRecord;
use App\Models\Device;
use App\Models\Tenant;
use App\Models\User;
use App\Services\BodyStore;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Storage;
use Tests\TestCase;

class DeleteUserDataTest extends TestCase
{
    use RefreshDatabase;

    private Tenant $tenant;

    private Device $device;

    private User $kim;

    private User $other;

    protected function setUp(): void
    {
        parent::setUp();
        Storage::fake('s3');
        app(TenantContext::class)->set(null);

        $this->tenant = Tenant::create(['name' => 'Acme', 'slug' => 'acme']);
        [$this->device] = Device::provision($this->tenant->id, 'mac', 'darwin');
        $this->kim = User::create(['tenant_id' => $this->tenant->id, 'name' => 'Kim', 'email' => 'kim@example.com', 'password' => 'x']);
        $this->other = User::create(['tenant_id' => $this->tenant->id, 'name' => 'Sam', 'email' => 'sam@example.com', 'password' => 'x']);
    }

    private function aiSession(User $user, string $day): AiSession
    {
        return AiSession::withoutGlobalScope('tenant')->create([
            'tenant_id' => $this->tenant->id, 'device_id' => $this->device->id, 'user_id' => $user->id,
            'tool' => 'claude-code', 'started_at' => $day, 'ended_at' => $day, 'interaction_count' => 0,
        ]);
    }

    private function row(AiSession $session, string $kind, string $at, string $text = 'text'): AiInteraction
    {
        $id = bin2hex(random_bytes(8));
        $bodies = app(BodyStore::class);

        return AiInteraction::withoutGlobalScope('tenant')->create([
            'tenant_id' => $this->tenant->id, 'device_id' => $this->device->id, 'user_id' => $session->user_id,
            'ai_session_id' => $session->id, 'event_id' => $id, 'host' => 'api.anthropic.com', 'tool' => 'claude-code',
            'kind' => $kind, 'automated' => $kind !== 'human', 'answer_chars' => 5,
            'prompt_object' => $bodies->put($this->tenant->id, $id, 'prompt', $text),
            'answer_object' => $bodies->put($this->tenant->id, $id, 'answer', 'answer'),
            'occurred_at' => $at,
        ]);
    }

    private function left(User $user): array
    {
        return AiInteraction::withoutGlobalScope('tenant')->where('user_id', $user->id)->orderBy('id')->pluck('id')->all();
    }

    public function test_an_admin_deletes_one_prompt_with_its_answer_from_the_dashboard(): void
    {
        $s = $this->aiSession($this->kim, '2026-09-20 10:00');
        $a = $this->row($s, 'human', '2026-09-20 10:00');
        $b = $this->row($s, 'human', '2026-09-20 10:05', 'my token is aiul_...');
        $bAnswer = $this->row($s, 'agent', '2026-09-20 10:06');
        $c = $this->row($s, 'human', '2026-09-20 10:10');

        $as = function (string $role) {
            $user = User::create(['tenant_id' => $this->tenant->id, 'name' => $role, 'email' => $role.'@example.com', 'password' => 'x', 'role' => $role]);
            ConsentRecord::withoutGlobalScope('tenant')->create([
                'tenant_id' => $this->tenant->id, 'user_id' => $user->id, 'kind' => ConsentRecord::KIND_CAPTURE,
                'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
            ]);

            return $this->actingAs($user);
        };

        $as(User::ROLE_MANAGER)->delete("/usage/{$b->id}")->assertForbidden();
        $this->assertCount(4, $this->left($this->kim));

        // Deleting from the answer's page takes the prompt it answered too.
        $as(User::ROLE_ADMIN)->delete("/usage/{$bAnswer->id}")->assertRedirect(route('usage.session', $s->id));

        $this->assertSame([$a->id, $c->id], $this->left($this->kim));
        Storage::disk('s3')->assertMissing($b->prompt_object);
        $this->assertSame(2, $s->refresh()->interaction_count);

        $stranger = Tenant::create(['name' => 'Other', 'slug' => 'other']);
        $outsider = User::create(['tenant_id' => $stranger->id, 'name' => 'O', 'email' => 'o@example.com', 'password' => 'x', 'role' => User::ROLE_ADMIN]);
        ConsentRecord::withoutGlobalScope('tenant')->create([
            'tenant_id' => $stranger->id, 'user_id' => $outsider->id, 'kind' => ConsentRecord::KIND_CAPTURE,
            'policy_version' => config('aiul.consent_version'), 'granted_at' => now(),
        ]);
        $this->actingAs($outsider)->delete("/usage/{$a->id}");
        $this->assertSame([$a->id, $c->id], $this->left($this->kim), 'another tenant cannot delete');
    }

    public function test_last_n_takes_whole_turns_and_tidies_the_session(): void
    {
        $s = $this->aiSession($this->kim, '2026-09-20 10:00');
        $a = $this->row($s, 'human', '2026-09-20 10:00');
        $aStep = $this->row($s, 'agent', '2026-09-20 10:01');
        $b = $this->row($s, 'human', '2026-09-20 10:05');
        $bStep = $this->row($s, 'agent', '2026-09-20 10:06');
        $bAnswer = $this->row($s, 'agent', '2026-09-20 10:07');
        $sams = $this->row($this->aiSession($this->other, '2026-09-20 11:00'), 'human', '2026-09-20 11:00');

        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com', '--last' => 1, '--force' => true])->assertSuccessful();

        $this->assertSame([$a->id, $aStep->id], $this->left($this->kim), 'only the last turn goes');
        $this->assertSame([$sams->id], $this->left($this->other), 'nobody else is touched');
        Storage::disk('s3')->assertMissing($b->prompt_object);
        Storage::disk('s3')->assertMissing($bAnswer->answer_object);
        Storage::disk('s3')->assertExists($a->prompt_object);

        $s->refresh();
        $this->assertSame(2, $s->interaction_count);
        $this->assertSame('2026-09-20 10:01:00', $s->ended_at->format('Y-m-d H:i:s'));
    }

    public function test_by_date_is_inclusive_and_empty_sessions_go(): void
    {
        $old = $this->row($this->aiSession($this->kim, '2026-09-01 09:00'), 'human', '2026-09-01 09:00');
        $mid = $this->row($this->aiSession($this->kim, '2026-09-10 23:59'), 'human', '2026-09-10 23:59');
        $new = $this->row($this->aiSession($this->kim, '2026-09-20 09:00'), 'human', '2026-09-20 09:00');

        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com', '--from' => '2026-09-05', '--to' => '2026-09-10', '--force' => true])
            ->assertSuccessful();

        $this->assertSame([$old->id, $new->id], $this->left($this->kim));
        $this->assertNull(AiSession::withoutGlobalScope('tenant')->find($mid->ai_session_id));
    }

    public function test_all_removes_everything_but_keeps_the_account(): void
    {
        $this->row($this->aiSession($this->kim, '2026-09-01 09:00'), 'human', '2026-09-01 09:00');
        $this->row($this->aiSession($this->kim, '2026-09-02 09:00'), 'human', '2026-09-02 09:00');

        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com', '--all' => true])
            ->expectsConfirmation('Delete this permanently? It cannot be undone.', 'yes')
            ->assertSuccessful();

        $this->assertSame([], $this->left($this->kim));
        $this->assertSame(0, AiSession::withoutGlobalScope('tenant')->where('user_id', $this->kim->id)->count());
        $this->assertNotNull($this->kim->fresh());
    }

    public function test_it_refuses_unclear_requests_and_dry_run_changes_nothing(): void
    {
        $this->row($this->aiSession($this->kim, '2026-09-01 09:00'), 'human', '2026-09-01 09:00');

        $this->artisan('aiul:delete-user-data', ['email' => 'nobody@example.com', '--all' => true])->assertFailed();
        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com'])->assertFailed();
        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com', '--all' => true, '--last' => 2])->assertFailed();
        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com', '--last' => 'five'])->assertFailed();
        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com', '--from' => '01/09/2026'])->assertFailed();
        $this->artisan('aiul:delete-user-data', ['email' => 'kim@example.com', '--all' => true, '--dry-run' => true])->assertSuccessful();

        $this->assertCount(1, $this->left($this->kim));
    }
}
