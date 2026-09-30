<?php

namespace Pm\Http;

use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Validation\Rule;
use Illuminate\Validation\ValidationException;
use Inertia\Inertia;
use Inertia\Response;
use Pm\Models\Project;
use Pm\Models\ProjectRemote;

class ProjectController
{
    /** /pm opens the first project's board, or the project list when there is none. */
    public function home(): RedirectResponse
    {
        $first = Project::orderBy('name')->first();

        return $first ? redirect()->route('pm.board', $first) : redirect()->route('pm.projects.index');
    }

    public function index(Request $request): Response
    {
        return Inertia::render('Pm/Projects', [
            'projects' => Project::with(['remotes:id,project_id,remote', 'sprints' => fn ($q) => $q->orderByDesc('start_date')])
                ->withCount(['tasks', 'tasks as open_count' => fn ($q) => $q->where('status', '!=', 'done')])
                ->orderBy('name')
                ->get()
                ->map(fn (Project $p) => [
                    'id' => $p->id, 'name' => $p->name, 'key' => $p->key,
                    'tasks' => $p->tasks_count, 'open' => $p->open_count,
                    'remotes' => $p->remotes->pluck('remote'),
                    'sprints' => $p->sprints->map->only(['id', 'name', 'start_date', 'end_date']),
                ]),
            'canManage' => $request->user()->isManager(),
        ]);
    }

    public function store(Request $request): RedirectResponse
    {
        abort_unless($request->user()->isManager(), 403);
        $tenant = $request->user()->tenant_id;
        $data = $request->validate([
            'name' => ['required', 'string', 'max:255'],
            // The prefix of every task key, AAY in AAY-42.
            'key' => ['required', 'regex:/^[A-Z]{2,6}$/', Rule::unique('pm_projects')->where('tenant_id', $tenant)],
            'remotes' => ['nullable', 'string', 'max:5000'],
        ], ['key.regex' => 'Use 2 to 6 capital letters, like AAY.']);

        $project = Project::create(['name' => $data['name'], 'key' => $data['key']]);
        $this->saveRemotes($project, $data['remotes'] ?? '');

        return redirect()->route('pm.board', $project);
    }

    public function update(Request $request, Project $project): RedirectResponse
    {
        abort_unless($request->user()->isManager(), 403);
        $data = $request->validate([
            'name' => ['required', 'string', 'max:255'],
            'remotes' => ['nullable', 'string', 'max:5000'],
        ]);

        $project->update(['name' => $data['name']]);
        $this->saveRemotes($project, $data['remotes'] ?? '');

        return back();
    }

    public function storeSprint(Request $request, Project $project): RedirectResponse
    {
        abort_unless($request->user()->isManager(), 403);
        $data = $request->validate([
            'name' => ['required', 'string', 'max:255'],
            'start_date' => ['required', 'date'],
            'end_date' => ['required', 'date', 'after_or_equal:start_date'],
        ]);

        $project->sprints()->create($data);

        return back();
    }

    /**
     * Replace a project's git remotes with the ones typed, one per line. They are
     * stored the way the agent sends them (host/path), so pasting a clone URL in
     * any form works.
     */
    private function saveRemotes(Project $project, string $text): void
    {
        $remotes = collect(preg_split('/[\s,]+/', $text, -1, PREG_SPLIT_NO_EMPTY))
            ->map(fn ($r) => self::normaliseRemote($r))
            ->filter()
            ->unique();

        $taken = ProjectRemote::whereIn('remote', $remotes)->where('project_id', '!=', $project->id)->pluck('remote');
        if ($taken->isNotEmpty()) {
            throw ValidationException::withMessages(['remotes' => 'Already used by another project: '.$taken->join(', ')]);
        }

        $project->remotes()->whereNotIn('remote', $remotes)->delete();
        foreach ($remotes as $remote) {
            $project->remotes()->firstOrCreate(['remote' => $remote]);
        }
    }

    /**
     * The PHP twin of the agent's NormaliseRemote (agent/internal/tasks): clone
     * URLs in any form become "host/path", e.g. "https://u:token@GitHub.com/Org/Repo.git"
     * and "git@github.com:Org/Repo.git" both become "github.com/Org/Repo". Also
     * accepts the bare "github.com/Org/Repo" people copy from a browser. Returns
     * null for anything that is not a shared remote.
     */
    public static function normaliseRemote(string $raw): ?string
    {
        $s = trim($raw);
        if (str_contains($s, '://')) {
            [$scheme, $rest] = explode('://', $s, 2);
            if (! in_array(strtolower($scheme), ['https', 'http', 'ssh', 'git', 'git+ssh', 'ssh+git'], true)) {
                return null;
            }
            [$host, $path] = array_pad(explode('/', $rest, 2), 2, '');
        } else {
            $colon = strpos($s, ':');
            $slash = strpos($s, '/');
            if ($colon !== false && $colon > 1 && ($slash === false || $colon < $slash)) {
                [$host, $path] = [substr($s, 0, $colon), substr($s, $colon + 1)]; // user@host:path
            } elseif ($slash !== false && preg_match('/^[a-z0-9-]+(\.[a-z0-9-]+)+$/i', substr($s, 0, $slash))) {
                [$host, $path] = [substr($s, 0, $slash), substr($s, $slash + 1)]; // github.com/org/repo
            } else {
                return null;
            }
        }

        $at = strrpos($host, '@'); // credentials: up to the LAST "@"
        if ($at !== false) {
            $host = substr($host, $at + 1);
        }
        $host = strtolower(explode(':', $host)[0]); // no port
        $path = trim($path, '/');
        if (str_ends_with($path, '.git')) {
            $path = substr($path, 0, -4);
        }

        return $host !== '' && $path !== '' ? "{$host}/{$path}" : null;
    }
}
