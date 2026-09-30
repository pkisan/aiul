<?php

namespace Pm\Http;

use App\Models\User;
use App\Services\BodyStore;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\Rule;
use Inertia\Inertia;
use Inertia\Response;
use Pm\Linking\Linker;
use Pm\Metrics\Insights;
use Pm\Models\Project;
use Pm\Models\Sprint;
use Pm\Models\Task;
use Pm\Models\WorkPeriod;

class TaskController
{
    /**
     * Five columns for one project. ?sprint=ID or ?sprint=all; by default the
     * sprint running today, else all. Backlog shows every backlog task: backlog
     * work is not in a sprint yet. No task open: `task` and `trail` are null.
     */
    public function board(Request $request, Project $project): Response
    {
        return Inertia::render('Pm/Board', $this->boardProps($request, $project) + ['task' => null, 'trail' => null]);
    }

    /**
     * The board props, each lazy: opening or closing a task from the board is
     * a partial reload of `task` and `trail`, and these are not recomputed.
     */
    private function boardProps(Request $request, Project $project): array
    {
        $sprints = $project->sprints()->orderByDesc('start_date')->get(['id', 'name', 'start_date', 'end_date']);
        $current = $sprints->first(fn (Sprint $s) => today()->between($s->start_date, $s->end_date));
        $sprint = $request->query('sprint', $current?->id ?? 'all');
        $sprint = $sprint === 'all' ? 'all' : ($sprints->firstWhere('id', (int) $sprint)?->id ?? 'all');

        return [
            'project' => fn () => $project->only(['id', 'name', 'key']),
            'projects' => fn () => Project::orderBy('name')->get(['id', 'name', 'key']),
            'sprints' => fn () => $sprints,
            'sprint' => fn () => $sprint,
            'tasks' => fn () => $project->tasks()
                ->with('assignee:id,name')
                ->withCount(['aiLinks as ai_sessions'])
                ->when($sprint !== 'all', fn ($q) => $q->where(fn ($q) => $q->where('sprint_id', $sprint)->orWhere('status', 'backlog')))
                ->orderBy('number')
                ->get()
                ->map(fn (Task $t) => $this->card($t, $project)),
            'statuses' => fn () => Task::STATUSES,
            'people' => fn () => $this->people(),
        ];
    }

    public function store(Request $request, Project $project): RedirectResponse
    {
        $data = $request->validate($this->rules($project, $request->user()) + [
            'title' => ['required', 'string', 'max:255'],
        ]);
        $data['status'] ??= 'todo';

        DB::transaction(function () use ($project, $data) {
            // The lock makes two people adding a task at once get 43 and 44, not 43 twice.
            Project::whereKey($project->id)->lockForUpdate()->first();
            $project->tasks()->create($data + [
                'number' => $project->nextNumber(),
                'started_at' => in_array($data['status'], ['in_progress', 'in_review', 'done'], true) ? now() : null,
                'completed_at' => $data['status'] === 'done' ? now() : null,
            ]);
        });
        $this->relink($data['assignee_id'] ?? null);

        return back();
    }

    /**
     * A task opens as a slide-over on its project's board, so /pm/tasks/{id}
     * renders the board with the task open. A shared link lands on the same view.
     */
    public function show(Request $request, Task $task, Insights $insights, BodyStore $bodies): Response
    {
        $task->load(['project', 'sprint', 'assignee:id,name', 'events' => fn ($q) => $q->latest('occurred_at')]);

        return Inertia::render('Pm/Board', $this->boardProps($request, $task->project) + [
            'task' => function () use ($task) {
                $names = User::whereIn('id', $task->events->pluck('user_id')->filter())->pluck('name', 'id');

                return $this->card($task->loadCount('aiLinks as ai_sessions'), $task->project) + [
                    'description' => $task->description,
                    'project_id' => $task->project_id,
                    'sprint_id' => $task->sprint_id,
                    'started_at' => $task->started_at,
                    'completed_at' => $task->completed_at,
                    'created_at' => $task->created_at,
                    // Sprints of the task's own project (the board may show another after a move).
                    'sprints' => $task->project->sprints()->orderByDesc('start_date')->get(['id', 'name']),
                    'events' => $task->events->map(fn ($e) => [
                        'id' => $e->id, 'field' => $e->field, 'from' => $e->from, 'to' => $e->to,
                        'at' => $e->occurred_at, 'by' => $names[$e->user_id] ?? null,
                    ]),
                ];
            },
            'trail' => fn () => $insights->trail($task, $request->user(), $bodies),
        ]);
    }

    public function update(Request $request, Task $task): RedirectResponse
    {
        $data = $request->validate($this->rules($task->project, $request->user()) + [
            'title' => ['sometimes', 'required', 'string', 'max:255'],
        ]);
        $before = $task->assignee_id;
        $task->applyChanges($data, $request->user());
        $this->relink($before, $task->assignee_id);

        return back();
    }

    /**
     * Move a task to another project. It gets a key there (its old one back if
     * it was there before) and keeps every earlier key, so branches named after
     * them still link. Its sprint belonged to the old project, so it is
     * cleared. AI links point at the task, not the project, so every prompt
     * follows without being touched (D20).
     */
    public function move(Request $request, Task $task): RedirectResponse
    {
        $data = $request->validate([
            'project_id' => ['required', Rule::exists('pm_projects', 'id')->where('tenant_id', $request->user()->tenant_id), Rule::notIn([$task->project_id])],
        ], ['project_id.not_in' => 'The task is already in that project.']);

        DB::transaction(function () use ($task, $data, $request) {
            $target = Project::whereKey($data['project_id'])->lockForUpdate()->firstOrFail();
            $from = $task->key;

            // Back to a project it was in before: take its old number again.
            $old = $task->keys()->where('key', 'like', $target->key.'-%')->value('key');
            $number = $old ? (int) substr($old, strlen($target->key) + 1) : $target->nextNumber();

            $task->update(['project_id' => $target->id, 'number' => $number, 'sprint_id' => null]);
            $task->setRelation('project', $target);
            if (! $old) {
                $task->keys()->create(['key' => $task->key]);
            }
            $task->events()->create([
                'user_id' => $request->user()->id, 'field' => 'project',
                'from' => $from, 'to' => $task->key, 'occurred_at' => now(),
            ]);
        });
        // A suggestion scoped to the old project's repository may no longer fit.
        $this->relink($task->assignee_id);

        return back();
    }

    /**
     * "Start working": this becomes my active task. AI sessions I start from
     * now on link to it (Phase 3). An untouched task moves to in progress and an
     * unassigned one becomes mine.
     */
    public function start(Request $request, Task $task): RedirectResponse
    {
        $me = $request->user();
        abort_if($task->assignee_id && $task->assignee_id !== $me->id, 403, 'This task is assigned to someone else.');

        DB::transaction(function () use ($task, $me) {
            WorkPeriod::where('user_id', $me->id)->whereNull('ended_at')->update(['ended_at' => now()]);
            WorkPeriod::create(['user_id' => $me->id, 'task_id' => $task->id, 'started_at' => now()]);

            $changes = array_filter([
                'status' => in_array($task->status, ['backlog', 'todo'], true) ? 'in_progress' : null,
                'assignee_id' => $task->assignee_id ? null : $me->id,
            ]);
            if ($changes) {
                $task->applyChanges($changes, $me);
            }
        });
        $this->relink($me->id);

        return back();
    }

    public function stop(Request $request): RedirectResponse
    {
        WorkPeriod::where('user_id', $request->user()->id)->whereNull('ended_at')->update(['ended_at' => now()]);
        $this->relink($request->user()->id);

        return back();
    }

    /** Who a task change affects: their recent AI sessions may now point elsewhere. */
    private function relink(?int ...$userIds): void
    {
        foreach (array_unique(array_filter($userIds)) as $id) {
            app(Linker::class)->relinkUser($id);
        }
    }

    /** What a board card and the task page header show. */
    private function card(Task $t, Project $project): array
    {
        return [
            'id' => $t->id, 'key' => $project->key.'-'.$t->number, 'title' => $t->title,
            'status' => $t->status, 'sprint_id' => $t->sprint_id,
            'assignee' => $t->assignee?->only(['id', 'name']),
            'ai_sessions' => $t->ai_sessions ?? 0,
        ];
    }

    /** Fields shared by create and edit. Assignee and sprint must be in this tenant and project. */
    private function rules(Project $project, User $me): array
    {
        return [
            'description' => ['sometimes', 'nullable', 'string', 'max:20000'],
            'status' => ['sometimes', Rule::in(Task::STATUSES)],
            'assignee_id' => ['sometimes', 'nullable', Rule::exists('users', 'id')->where('tenant_id', $me->tenant_id)],
            'sprint_id' => ['sometimes', 'nullable', Rule::exists('pm_sprints', 'id')->where('project_id', $project->id)],
        ];
    }

    private function people()
    {
        return User::where('tenant_id', auth()->user()->tenant_id)->orderBy('name')->get(['id', 'name']);
    }
}
