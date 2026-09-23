<?php

namespace App\Http\Controllers\Api;

use App\Http\Controllers\Controller;
use App\Jobs\ScoreInteraction;
use App\Models\AiInteraction;
use App\Models\AiSession;
use App\Models\Device;
use App\Services\BodyStore;
use App\Services\PromptText;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\Validator;

/**
 * Receives batches of events from the agent.
 *
 * The contract that matters: the reply lists the event ids that were STORED, and
 * the agent deletes only those from its spool. So a partial failure loses nothing
 * — whatever is not confirmed is sent again — and a batch replayed after a network
 * timeout is harmless, because event ids are unique per tenant.
 */
class EventIngestionController extends Controller
{
    public function __construct(private readonly BodyStore $bodies) {}

    public function store(Request $request): JsonResponse
    {
        /** @var Device $device */
        $device = $request->attributes->get('device');

        $data = $request->validate([
            'device_id' => ['nullable', 'string', 'max:255'],
            'sent_at' => ['nullable', 'date'],
            'events' => ['required', 'array', 'max:200'],
        ]);

        $accepted = [];
        $rejected = [];

        foreach ($data['events'] as $raw) {
            $result = $this->storeEvent($device, is_array($raw) ? $raw : []);

            if ($result === null) {
                // Keep the id if we can see one, so the agent can report it.
                $rejected[] = ['id' => $raw['id'] ?? null, 'reason' => 'invalid'];

                continue;
            }

            $accepted[] = $result;
        }

        return response()->json([
            'accepted' => $accepted,
            'rejected' => $rejected,
        ]);
    }

    /**
     * Store one event. Returns its id when stored, or null when it was malformed.
     *
     * A malformed event is REJECTED rather than failing the batch: one bad record
     * must not block every good one behind it.
     */
    private function storeEvent(Device $device, array $event): ?string
    {
        $validator = Validator::make($event, [
            'id' => ['required', 'string', 'max:64'],
            'host' => ['required', 'string', 'max:255'],
            'time' => ['required', 'date'],
            'path' => ['nullable', 'string', 'max:2048'],
            'status' => ['nullable', 'integer', 'min:0', 'max:999'],
            'tool' => ['nullable', 'string', 'max:100'],
            'model' => ['nullable', 'string', 'max:150'],
            'parser' => ['nullable', 'string', 'max:50'],
            'prompt' => ['nullable', 'string'],
            'answer' => ['nullable', 'string'],
            'system' => ['nullable', 'string'],
            'task_id' => ['nullable', 'string', 'max:64'],
            'repo' => ['nullable', 'string', 'max:1024'],
            'branch' => ['nullable', 'string', 'max:255'],
            'streamed' => ['nullable', 'boolean'],
            'automated' => ['nullable', 'boolean'],
            'redacted' => ['nullable', 'array'],
        ]);

        if ($validator->fails()) {
            return null;
        }

        // Idempotency: a batch sent twice must not create two rows. Returning the
        // id as accepted lets the agent delete its copy either way.
        $existing = AiInteraction::where('event_id', $event['id'])->first();

        if ($existing) {
            return $existing->event_id;
        }

        // The agent sends RFC 3339 with the device's offset ("...T15:29:42+05:30").
        // Columns are plain timestamps and the application runs in UTC, so the
        // offset has to be applied here — without ->utc() the local wall clock is
        // stored as if it were UTC and every screen shows the device's offset
        // added on top of it.
        $occurredAt = Carbon::parse($event['time'])->utc();

        // Only what the person typed: the tool's own wrappers never reach storage.
        $event['prompt'] = PromptText::clean($event['prompt'] ?? null);
        $session = $this->sessionFor($device, $event, $occurredAt);

        $interaction = new AiInteraction([
            'tenant_id' => $device->tenant_id,
            'device_id' => $device->id,
            'user_id' => $device->user_id,
            'ai_session_id' => $session->id,
            'event_id' => $event['id'],
            'host' => $event['host'],
            'path' => $event['path'] ?? null,
            'status' => $event['status'] ?? null,
            'tool' => $event['tool'] ?? null,
            'model' => $event['model'] ?? null,
            'parser' => $event['parser'] ?? null,
            'task_id' => $event['task_id'] ?? null,
            'repo' => $event['repo'] ?? null,
            'branch' => $event['branch'] ?? null,
            'prompt_chars' => mb_strlen((string) ($event['prompt'] ?? '')),
            'answer_chars' => mb_strlen((string) ($event['answer'] ?? '')),
            'prompt_tokens' => $event['prompt_tokens'] ?? 0,
            'response_tokens' => $event['response_tokens'] ?? 0,
            'request_bytes' => $event['request_bytes'] ?? 0,
            'response_bytes' => $event['response_bytes'] ?? 0,
            'duration_ms' => $event['duration_ms'] ?? 0,
            'streamed' => (bool) ($event['streamed'] ?? false),
            'automated' => (bool) ($event['automated'] ?? false),
            // "human", "agent" or "utility" — see the migration. Older agents
            // send no kind at all, and a null kind is honest about that.
            'kind' => $event['kind'] ?? null,
            'redacted' => $event['redacted'] ?? null,
            'redaction_rules_version' => $event['redaction_rules_version'] ?? 0,
            'allowlist_version' => $event['allowlist_version'] ?? 0,
            'occurred_at' => $occurredAt,
        ]);

        // Bodies go to object storage, encrypted; the row keeps only the key.
        $interaction->prompt_object = $this->bodies->put(
            $device->tenant_id, $event['id'], 'prompt', $event['prompt'] ?? null
        );
        $interaction->answer_object = $this->bodies->put(
            $device->tenant_id, $event['id'], 'answer', $event['answer'] ?? null
        );

        $interaction->save();

        $session->increment('interaction_count');
        $session->forceFill(['ended_at' => $occurredAt])->save();

        // Scoring is queued: ingestion must stay fast, and a scoring bug must
        // never cost us an event.
        ScoreInteraction::dispatch($interaction->id, $device->tenant_id);

        return $interaction->event_id;
    }

    /**
     * Find or start the session this event belongs to.
     *
     * A session is one continuous stretch of work: same device, tool and task,
     * with no gap longer than the idle window. That is what makes "AI time per
     * task" mean something rather than counting wall-clock time with lunch in it.
     */
    private function sessionFor(Device $device, array $event, Carbon $occurredAt): AiSession
    {
        $idleWindow = now()->parse($occurredAt)->subMinutes(config('aiul.session_idle_minutes', 30));

        // Grouped by the checkout, not the ticket. Two pieces of work in
        // different repositories within the idle window used to land in one
        // session, because both had a null task_id — so a session said "Aayatti"
        // while half its interactions came from another project entirely.
        $session = AiSession::where('device_id', $device->id)
            ->where('tool', $event['tool'] ?? null)
            ->where('repo', $event['repo'] ?? null)
            ->where('ended_at', '>=', $idleWindow)
            ->latest('ended_at')
            ->first();

        if ($session) {
            return $session;
        }

        return AiSession::create([
            'tenant_id' => $device->tenant_id,
            'device_id' => $device->id,
            'user_id' => $device->user_id,
            'tool' => $event['tool'] ?? null,
            'task_id' => $event['task_id'] ?? null,
            'repo' => $event['repo'] ?? null,
            'branch' => $event['branch'] ?? null,
            'started_at' => $occurredAt,
            'ended_at' => $occurredAt,
            'interaction_count' => 0,
        ]);
    }
}
