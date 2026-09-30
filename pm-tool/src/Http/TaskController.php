<?php

namespace Pm\Http;

use App\Models\User;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\Rule;
use Inertia\Inertia;
use Inertia\Response;
use Pm\Models\Project;
use Pm\Models\Sprint;
use Pm\Models\Task;
use Pm\Models\WorkPeriod;

class TaskController
{
    /**
     * Five columns for one project. ?sprint=ID or ?sprint=all; by default the
     * sprint running today, else all. Backlog shows every backlog task: backlog
     * work is not in a sprint yet.
     */
    public function board(Request $request, Project $project): Response
    {
        $sprints = $project->sprints()->orderByDesc('start_date')->get(['id', 'name', 'start_date', 'end_date']);
        $current = $sprints->first(fn (Sprint $s) => today()->between($s->start_date, $s->end_date));
        $sprint = $request->query('sprint', $current?->id ?? 'all');
        $sprint = $sprint === 'all' ? 'all' : ($sprints->firstWhere('id', (int) $sprint)?->id ?? 'all');

        $tasks = $project->tasks()
            ->with('assignee:id,name')
            ->withCount(['aiLinks as ai_sessions'])
            ->when($sprint !== 'all', fn ($q) => $q->where(fn ($q) => $q->where('sprint_id', $sprint)->orWhere('status', 'backlog')))
            ->orderBy('number')
            ->get()
            ->map(fn (Task $t) => $this->card($t, $project));

        return Inertia::render('Pm/Board', [
            'project' => $project->only(['id', 'name', 'key']),
            'projects' => Project::orderBy('name')->get(['id', 'name', 'key']),
            'sprints' => $sprints,
            'sprint' => $sprint,
            'tasks' => $tasks,
            'statuses' => Task::STATUSES,
            'people' => $this->people(),
        ]);
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
                'number' => $project->tasks()->max('number') + 1,
                'started_at' => in_array($data['status'], ['in_progress', 'in_review', 'done'], true) ? now() : null,
                'completed_at' => $data['status'] === 'done' ? now() : null,
            ]);
        });

        return back();
    }

    public function show(Request $request, Task $task): Response
    {
        $task->load(['project', 'sprint', 'assignee:id,name', 'events' => fn ($q) => $q->latest('occurred_at')]);
        $names = User::whereIn('id', $task->events->pluck('user_id')->filter())->pluck('name', 'id');

        return Inertia::render('Pm/Task', [
            'task' => $this->card($task->loadCount('aiLinks as ai_sessions'), $task->project) + [
                'description' => $task->description,
                'sprint_id' => $task->sprint_id,
                'sprint' => $task->sprint?->name,
                'started_at' => $task->started_at,
                'completed_at' => $task->completed_at,
                'created_at' => $task->created_at,
            ],
            'project' => $task->project->only(['id', 'name', 'key']),
            'events' => $task->events->map(fn ($e) => [
                'id' => $e->id, 'field' => $e->field, 'from' => $e->from, 'to' => $e->to,
                'at' => $e->occurred_at, 'by' => $names[$e->user_id] ?? null,
            ]),
            'sprints' => $task->project->sprints()->orderByDesc('start_date')->get(['id', 'name']),
            'statuses' => Task::STATUSES,
            'people' => $this->people(),
        ]);
    }

    public function update(Request $request, Task $task): RedirectResponse
    {
        $data = $request->validate($this->rules($task->project, $request->user()) + [
            'title' => ['sometimes', 'required', 'string', 'max:255'],
        ]);
        $task->applyChanges($data, $request->user());

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

        return back();
    }

    public function stop(Request $request): RedirectResponse
    {
        WorkPeriod::where('user_id', $request->user()->id)->whereNull('ended_at')->update(['ended_at' => now()]);

        return back();
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
