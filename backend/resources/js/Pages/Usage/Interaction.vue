<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { count, when } from '@/Components/Usage/format';
import Message from '@/Components/Usage/Message.vue';
import { Head, Link } from '@inertiajs/vue3';

const props = defineProps({
    interaction: Object,
    score: Object,
    canViewRaw: Boolean,
    // Present only when canViewRaw: the view was audit-logged before they were sent.
    prompt: String,
    answer: String,
    promptState: String,
    answerState: String,
});

// Who caused the request decides whose words the prompt is.
const kind = props.interaction.kind ?? (props.interaction.automated ? 'agent' : 'human');
const promptWho = { human: 'you', agent: 'agent', utility: 'tool' }[kind] ?? 'you';
const promptLabel = { human: 'Prompt typed by the person', agent: 'Agent step (tool result fed back)', utility: "Tool's own call" }[kind];
const missing = (state, what) =>
    state === 'purged' ? `This ${what} was purged by the retention policy.` : `No ${what} text was captured.`;
</script>

<template>
    <Head title="Interaction" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">Interaction</h2>
                <Tag :label="promptLabel" :tone="kind === 'human' ? 'green' : 'gray'" />
                <Link
                    v-if="interaction.ai_session_id"
                    :href="route('usage.session', interaction.ai_session_id)"
                    class="ml-auto text-sm text-gray-500 hover:text-gray-900"
                >Whole conversation →</Link>
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-4xl space-y-4 px-4 sm:px-6 lg:px-8">
                <Panel title="Conversation" subtitle="Opening this page is recorded in the audit log">
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

                <Panel v-if="score" title="Prompt quality" :subtitle="`rubric v${score.rubric_version}`">
                    <template #actions>
                        <Score :value="score.score" class="text-base" />
                    </template>
                    <ul class="divide-y divide-gray-100">
                        <li v-for="(dimension, name) in score.dimensions" :key="name" class="flex gap-4 px-5 py-3">
                            <Score :value="dimension.score" class="w-10 shrink-0 text-right" />
                            <div>
                                <div class="text-sm font-medium capitalize text-gray-900">
                                    {{ String(name).replaceAll('_', ' ') }}
                                </div>
                                <p class="text-sm text-gray-600">{{ dimension.reason }}</p>
                            </div>
                        </li>
                    </ul>
                </Panel>

            </div>
        </div>
    </AuthenticatedLayout>
</template>
