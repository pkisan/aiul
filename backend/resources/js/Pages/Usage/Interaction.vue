<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { count, when } from '@/Components/Usage/format';
import { Head, Link, usePage } from '@inertiajs/vue3';
import { computed, ref } from 'vue';

const props = defineProps({ interaction: Object, score: Object, canViewRaw: Boolean });

// Someone else's words require a reason; the server bounces a reason-less
// visit back with an error, which used to look like a dead button.
const reason = ref('');
const errors = computed(() => usePage().props.errors);
const rawUrl = computed(() => {
    const base = route('usage.raw', props.interaction.id);
    return reason.value.trim() ? `${base}?reason=${encodeURIComponent(reason.value.trim())}` : base;
});
</script>

<template>
    <Head title="Interaction" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">Interaction</h2>
                <Tag :label="interaction.automated ? 'automated follow-up' : 'human prompt'"
                     :tone="interaction.automated ? 'gray' : 'green'" />
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-4xl space-y-4 px-4 sm:px-6 lg:px-8">
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

                <Panel title="Prompt text" subtitle="Every read is written to the audit log">
                    <div v-if="canViewRaw" class="space-y-3 px-5 py-4">
                        <div>
                            <label class="mb-1 block text-sm text-gray-600" for="reason">
                                Reason for opening (optional)
                            </label>
                            <input
                                id="reason"
                                v-model="reason"
                                type="text"
                                placeholder="e.g. support investigation"
                                class="w-full rounded-lg border-gray-300 text-sm shadow-sm focus:border-gray-400 focus:ring-gray-400"
                            />
                            <p v-if="errors.reason" class="mt-1 text-sm text-rose-600">{{ errors.reason }}</p>
                        </div>
                        <Link
                            :href="rawUrl"
                            class="inline-flex items-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-700"
                        >Open the prompt text</Link>
                    </div>
                    <p v-else class="px-5 py-8 text-center text-sm text-gray-500">
                        You do not have permission to read the prompt text.
                    </p>
                </Panel>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
