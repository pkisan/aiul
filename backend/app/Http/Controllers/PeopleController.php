<?php

namespace App\Http\Controllers;

use App\Models\AiInteraction;
use App\Models\Device;
use App\Models\User;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Str;
use Illuminate\Validation\Rule;
use Inertia\Inertia;
use Inertia\Response;

/**
 * The admin's list of accounts. There is no public sign-up: everyone who can
 * sign in was added here (or is the super admin the seeder made).
 *
 * No delete button, on purpose. Deleting a user cascades to their consent
 * records and leaves their usage unassigned. Resetting the password locks a
 * leaver out just as well.
 */
class PeopleController extends Controller
{
    public function index(Request $request): Response
    {
        abort_unless($request->user()->isAdmin(), 403);

        // Queries run under the tenant scope set by SetTenantFromUser, except
        // User, which is filtered by hand.
        $lastActive = AiInteraction::query()
            ->whereNotNull('user_id')
            ->selectRaw('user_id, max(occurred_at) as at')
            ->groupBy('user_id')
            ->pluck('at', 'user_id');
        $devices = Device::query()
            ->whereNotNull('user_id')
            ->where('revoked', false)
            ->orderBy('hostname')
            ->get(['id', 'user_id', 'hostname', 'platform', 'last_seen_at'])
            ->groupBy('user_id');

        return Inertia::render('People', [
            'people' => User::where('tenant_id', $request->user()->tenant_id)
                ->orderBy('name')
                ->get(['id', 'name', 'email', 'role', 'can_view_raw_prompts', 'created_at'])
                ->map(fn (User $u) => [
                    ...$u->only(['id', 'name', 'email', 'role', 'can_view_raw_prompts', 'created_at']),
                    'devices' => ($devices[$u->id] ?? collect())
                        ->map(fn ($d) => $d->only(['id', 'hostname', 'platform', 'last_seen_at']))->values(),
                    'last_active' => $lastActive[$u->id] ?? null,
                ]),
            // A new password or device token is shown once, straight after it is made.
            'issued' => session('issued'),
            'deviceToken' => session('deviceToken'),
            'endpoint' => url('/api/aiul/events'),
        ]);
    }

    public function store(Request $request): RedirectResponse
    {
        abort_unless($request->user()->isAdmin(), 403);

        $data = $request->validate([
            'name' => ['required', 'string', 'max:255'],
            'email' => ['required', 'string', 'lowercase', 'email', 'max:255', Rule::unique('users', 'email')],
            ...$this->accessRules(),
        ]);

        $password = Str::password(16);
        $user = User::create([
            ...$data,
            'tenant_id' => $request->user()->tenant_id,
            'can_view_raw_prompts' => $this->rawAllowed($data),
            'password' => $password,
        ]);
        $user->forceFill(['email_verified_at' => now()])->save();

        return back()->with('issued', ['email' => $user->email, 'password' => $password]);
    }

    public function update(Request $request, User $person): RedirectResponse
    {
        $this->authorizeTarget($request, $person);

        $data = $request->validate($this->accessRules());

        // An admin cannot demote themselves: that is how a tenant ends up with
        // nobody able to manage it.
        abort_if($person->is($request->user()) && $data['role'] !== User::ROLE_ADMIN, 422, 'You cannot remove your own admin role.');

        $person->update([
            'role' => $data['role'],
            'can_view_raw_prompts' => $this->rawAllowed($data),
        ]);

        return back();
    }

    /** A new random password, shown once. Also how a leaver is locked out. */
    public function resetPassword(Request $request, User $person): RedirectResponse
    {
        $this->authorizeTarget($request, $person);

        $password = Str::password(16);
        $person->update(['password' => $password, 'remember_token' => Str::random(60)]);

        return back()->with('issued', ['email' => $person->email, 'password' => $password]);
    }

    /**
     * A token for one of this person's machines, shown once. The same device
     * name again replaces its token, which is also how a leaked one is revoked.
     */
    public function issueDevice(Request $request, User $person): RedirectResponse
    {
        $this->authorizeTarget($request, $person);

        $data = $request->validate([
            'hostname' => $this->hostnameRules(),
            'platform' => ['required', Rule::in(Device::PLATFORMS)],
        ], $this->hostnameMessages());

        [, $token] = Device::provision($person->tenant_id, $data['hostname'], $data['platform'], $person->id);

        return back()->with('deviceToken', [
            'person' => $person->name,
            'hostname' => $data['hostname'],
            'platform' => $data['platform'],
            'token' => $token,
        ]);
    }

    /**
     * Give a device a new name. Only the label changes: the agent is found by
     * its token, so the token and everything already recorded stay as they are.
     */
    public function renameDevice(Request $request, User $person, int $device): RedirectResponse
    {
        $this->authorizeTarget($request, $person);

        $device = Device::withoutGlobalScope('tenant')
            ->where('tenant_id', $person->tenant_id)
            ->where('user_id', $person->id)
            ->findOrFail($device);

        // Unique per tenant: issuing a token looks a device up by its name, so two
        // devices with one name would make that ambiguous.
        $data = $request->validate([
            'hostname' => [
                ...$this->hostnameRules(),
                Rule::unique('devices', 'hostname')->where('tenant_id', $person->tenant_id)->ignore($device->id),
            ],
        ], $this->hostnameMessages() + ['hostname.unique' => 'Another device already has that name.']);

        $device->update(['hostname' => $data['hostname']]);

        return back();
    }

    /** Shown on pages and passed to shell commands by people: keep it plain. */
    private function hostnameRules(): array
    {
        return ['required', 'string', 'max:100', 'regex:/^[A-Za-z0-9][A-Za-z0-9._-]*$/'];
    }

    private function hostnameMessages(): array
    {
        return ['hostname.regex' => 'Use letters, numbers, dots, dashes or underscores only.'];
    }

    private function accessRules(): array
    {
        return [
            'role' => ['required', Rule::in([User::ROLE_MEMBER, User::ROLE_MANAGER, User::ROLE_ADMIN])],
            'can_view_raw_prompts' => ['boolean'],
        ];
    }

    /** Raw prompt access is an admin-only permission (see User::canViewRawPrompts). */
    private function rawAllowed(array $data): bool
    {
        return $data['role'] === User::ROLE_ADMIN && (bool) ($data['can_view_raw_prompts'] ?? false);
    }

    /** Admins manage their own tenant only; route binding alone does not check that. */
    private function authorizeTarget(Request $request, User $person): void
    {
        abort_unless($request->user()->isAdmin(), 403);
        abort_unless($person->tenant_id === $request->user()->tenant_id, 404);
    }
}
