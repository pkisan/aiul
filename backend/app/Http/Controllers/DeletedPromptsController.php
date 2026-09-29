<?php

namespace App\Http\Controllers;

use App\Models\AiInteraction;
use App\Models\User;
use App\Services\BodyStore;
use App\Services\PromptText;
use App\Services\UsageEraser;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\Log;
use Inertia\Inertia;
use Inertia\Response;

/**
 * Deleted prompts: what admins deleted from the dashboard, restorable until
 * aiul:purge-deleted removes it after config('aiul.trash_days'). Admins only.
 */
class DeletedPromptsController extends Controller
{
    public function index(Request $request, BodyStore $bodies): Response
    {
        abort_unless($request->user()->isAdmin(), 403);
        $canRead = $request->user()->canViewRawPrompts();
        $days = config('aiul.trash_days');

        // One row per deletion: the first row of the turn stands for it (the
        // typed prompt, when there was one). The tenant scope applies.
        $firsts = AiInteraction::onlyTrashed()
            ->whereNotNull('deletion_id')
            ->selectRaw('min(id) as id, deletion_id, count(*) as rows, max(deleted_at) as deleted_at, max(deleted_by) as deleted_by')
            ->groupBy('deletion_id')
            ->orderByDesc('deleted_at')
            ->paginate(25);

        $rows = AiInteraction::onlyTrashed()->with(['user:id,name'])
            ->whereIn('id', $firsts->pluck('id'))->get()->keyBy('id');
        $by = User::whereIn('id', $firsts->pluck('deleted_by')->filter())->pluck('name', 'id');

        return Inertia::render('Usage/Deleted', [
            'trashDays' => $days,
            'canRead' => $canRead,
            'deletions' => $firsts->through(function ($d) use ($rows, $by, $bodies, $canRead, $days) {
                $row = $rows[$d->id];

                return [
                    'deletion_id' => $d->deletion_id,
                    'rows' => $d->rows,
                    'person' => $row->user?->name,
                    'tool' => $row->tool,
                    'occurred_at' => $row->occurred_at,
                    'deleted_at' => $d->deleted_at,
                    'deleted_by' => $by[$d->deleted_by] ?? null,
                    'purge_at' => Carbon::parse($d->deleted_at)->addDays($days),
                    // Text only for admins who may read prompts at all.
                    'prompt' => $canRead ? mb_strimwidth((string) PromptText::clean($bodies->get($row->prompt_object)), 0, 240, '…') : null,
                ];
            }),
        ]);
    }

    public function restore(Request $request, string $deletion, UsageEraser $eraser): RedirectResponse
    {
        abort_unless($request->user()->isAdmin(), 403);
        $count = $eraser->restore($request->user()->tenant_id, $deletion);
        abort_if($count === 0, 404);

        Log::info('Deleted prompt restored', ['by_user_id' => $request->user()->id, 'deletion_id' => $deletion]);

        return back();
    }

    public function destroy(Request $request, string $deletion, UsageEraser $eraser): RedirectResponse
    {
        abort_unless($request->user()->isAdmin(), 403);
        $ids = $eraser->deletion($request->user()->tenant_id, $deletion)->pluck('id');
        abort_if($ids->isEmpty(), 404);
        $eraser->delete($request->user()->tenant_id, $ids);

        Log::info('Deleted prompt removed permanently', [
            'by_user_id' => $request->user()->id, 'deletion_id' => $deletion, 'interaction_ids' => $ids->all(),
        ]);

        return back();
    }
}
