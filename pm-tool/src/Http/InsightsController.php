<?php

namespace Pm\Http;

use App\Models\AiInteraction;
use App\Models\User;
use App\Services\BodyStore;
use App\Services\PromptText;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Response;
use Pm\Metrics\Insights;
use Pm\Metrics\Period;
use Pm\Models\TaskAiLink;

/** Team Pulse, Person, Tools & cost, and the prompt text behind a trail row. */
class InsightsController
{
    public function pulse(Request $request, Insights $insights): Response
    {
        abort_unless($request->user()->isManager(), 403);
        $period = Period::from($request->query('period'));

        return Inertia::render('Pm/Pulse', $insights->pulse($period) + [
            'period' => $period->toArray(), 'periods' => Period::options(), 'days' => $period->days(),
        ]);
    }

    /** One person's work. Themselves, or a manager: support, not surveillance. */
    public function person(Request $request, Insights $insights, ?User $user = null): Response
    {
        $user ??= $request->user();
        abort_unless($user->tenant_id === $request->user()->tenant_id, 404);
        abort_unless($user->is($request->user()) || $request->user()->isManager(), 403);
        $period = Period::from($request->query('period'));

        return Inertia::render('Pm/Person', $insights->person($user, $period) + [
            'person' => $user->only(['id', 'name']),
            'isMe' => $user->is($request->user()),
            'period' => $period->toArray(), 'periods' => Period::options(),
        ]);
    }

    public function tools(Request $request, Insights $insights): Response
    {
        abort_unless($request->user()->isManager(), 403);
        $period = Period::from($request->query('period'));

        return Inertia::render('Pm/Tools', $insights->tools($period) + [
            'period' => $period->toArray(), 'periods' => Period::options(),
        ]);
    }

    /** Full prompt and answer of one trail row, when expanded. Owner or manager, linked sessions only. */
    public function turn(Request $request, AiInteraction $interaction, BodyStore $bodies): JsonResponse
    {
        abort_unless(TaskAiLink::where('ai_session_id', $interaction->ai_session_id)->whereNotNull('task_id')->exists(), 404);
        $me = $request->user();
        abort_unless($interaction->user_id === $me->id || $me->isManager(), 403);

        return response()->json([
            'prompt' => PromptText::clean($bodies->get($interaction->prompt_object)),
            'answer' => $bodies->get($interaction->answer_object),
        ]);
    }
}
