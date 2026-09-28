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
 * No delete button, on purpose. Deleting a user cascades to their consent and
 * audit records, and an audit log that can be erased is not an audit log.
 * Resetting the password locks a leaver out just as well.
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
            ->selectRaw('user_id, count(*) as n')
            ->groupBy('user_id')
            ->pluck('n', 'user_id');

        return Inertia::render('People', [
            'people' => User::where('tenant_id', $request->user()->tenant_id)
                ->orderBy('name')
                ->get(['id', 'name', 'email', 'role', 'can_view_raw_prompts', 'created_at'])
                ->map(fn (User $u) => [
                    ...$u->only(['id', 'name', 'email', 'role', 'can_view_raw_prompts', 'created_at']),
                    'devices' => (int) ($devices[$u->id] ?? 0),
                    'last_active' => $lastActive[$u->id] ?? null,
                ]),
            // A new password is shown once, straight after it is made.
            'issued' => session('issued'),
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
