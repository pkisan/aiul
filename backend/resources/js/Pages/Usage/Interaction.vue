<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { count, when } from '@/Components/Usage/format';
import Message from '@/Components/Usage/Message.vue';
import { Head, Link, router } from '@inertiajs/vue3';

const props = defineProps({
    interaction: Object,
    canViewRaw: Boolean,
    canDelete: Boolean,
    person: String,
    // Present only when canViewRaw.
    prompt: String,
    answer: String,
    promptState: String,
    answerState: String,
});

// Who caused the request decides whose words the prompt is.
const kind = props.interaction.kind ?? (props.interaction.automated ? 'agent' : 'human');
const promptWho = { human: 'you', agent: 'agent', utility: 'tool' }[kind] ?? 'you';
const promptLabel = { human: 'Prompt typed by the person', agent: 'Agent step (tool result fed back)', utility: "Tool's own call" }[kind];
// Admins only. Takes the whole turn: the prompt, the agent's steps and the answer.
// Normally to Deleted prompts (restorable); "permanently" is for a leaked secret.
const destroy = (permanent) => {
    const message = permanent
        ? 'Delete this prompt, its agent steps and its answer PERMANENTLY? The text is erased now and cannot be restored.'
        : 'Move this prompt, its agent steps and its answer to Deleted prompts? An admin can restore it there until it is removed for good.';
    if (confirm(message)) {
        router.delete(route('usage.destroy', props.interaction.id), { data: { permanent } });
    }
};

const missing = (state, what) =>
    state === 'purged' ? `This ${what} was purged by the retention policy.` : `No ${what} text was captured.`;
</script>

<template>
    <Head title="Interaction" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.activity')" class="text-sm text-gray-500 hover:text-gray-900">← Activity</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">Interaction</h2>
                <Tag :label="promptLabel" :tone="kind === 'human' ? 'green' : 'gray'" />
                <Link
                    v-if="interaction.ai_session_id"
                    :href="route('usage.session', interaction.ai_session_id)"
                    class="ml-auto text-sm text-gray-500 hover:text-gray-900"
                >Whole conversation →</Link>
                <template v-if="canDelete">
                    <button
                        type="button"
                        class="text-sm text-rose-600 hover:underline dark:text-rose-400"
                        :class="{ 'ml-auto': !interaction.ai_session_id }"
                        @click="destroy(false)"
                    >Delete prompt</button>
                    <button
                        type="button"
                        class="text-sm text-gray-500 hover:text-rose-600 hover:underline"
                        title="For a secret that slipped past redaction: erased now, not restorable"
                        @click="destroy(true)"
                    >Delete permanently</button>
                </template>
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-4xl space-y-4 px-4 sm:px-6 lg:px-8">
                <Panel title="Conversation">
                    <div v-if="canViewRaw" class="space-y-3 bg-gray-50/60 px-5 py-5">
                        <div class="text-right text-[11px] font-medium uppercase tracking-wide text-gray-400">{{ promptLabel }}</div>
                        <Message :who="promptWho" :text="promptState === 'present' ? prompt : ''" :placeholder="missing(promptState, 'prompt')" />
                        <div class="text-[11px] font-medium uppercase tracking-wide text-gray-400">
                            Answer · {{ interaction.model ?? 'unknown model' }}
                        </div>
                        <Message who="assistant" :text="answerState === 'present' ? answer : ''" :placeholder="missing(answerState, 'answer')" />
                    </div>
                    <p v-else class="px-5 py-8 text-center text-sm text-gray-500">
                        You do not have permission to read the prompt text.
                    </p>
                </Panel>

                <Panel title="What was captured">
                    <dl class="grid grid-cols-2 gap-x-6 gap-y-3 px-5 py-4 text-sm sm:grid-cols-4">
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">Tool</dt>
                            <dd class="mt-0.5 text-gray-900">{{ interaction.tool ?? '—' }}</dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">AI account</dt>
                            <dd class="mt-0.5 truncate text-gray-900">
                                {{ interaction.account ?? person ?? '—' }}
                                <span v-if="!interaction.account && person" class="text-xs text-gray-400">(device's person)</span>
                            </dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">Model</dt>
                            <dd class="mt-0.5 text-gray-900">{{ interaction.model ?? '—' }}</dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">Task</dt>
                            <dd class="mt-0.5 text-gray-900">{{ interaction.task_id ?? 'untagged' }}</dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">Branch</dt>
                            <dd class="mt-0.5 truncate text-gray-900">{{ interaction.branch ?? '—' }}</dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">Tokens</dt>
                            <dd class="mt-0.5 tabular-nums text-gray-900">
                                {{ count(interaction.prompt_tokens) }} in / {{ count(interaction.response_tokens) }} out
                            </dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">Duration</dt>
                            <dd class="mt-0.5 tabular-nums text-gray-900">{{ count(interaction.duration_ms) }} ms</dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">When</dt>
                            <dd class="mt-0.5 text-gray-900">{{ when(interaction.occurred_at) }}</dd>
                        </div>
                        <div>
                            <dt class="text-xs uppercase tracking-wide text-gray-500">Masked</dt>
                            <dd class="mt-0.5 text-gray-900">
                                {{ interaction.redacted?.length ? interaction.redacted.join(', ') : 'nothing' }}
                            </dd>
                        </div>
                    </dl>
                </Panel>


            </div>
        </div>
    </AuthenticatedLayout>
</template>
