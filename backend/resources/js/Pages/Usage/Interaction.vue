<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
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
            <h2 class="text-xl font-semibold leading-tight text-gray-800">Interaction</h2>
        </template>

        <div class="py-8">
            <div class="mx-auto max-w-4xl space-y-4 px-4 sm:px-6 lg:px-8">
                <div class="rounded-lg bg-white p-4 text-sm shadow">
                    <dl class="grid grid-cols-2 gap-2">
                        <dt class="text-gray-500">Tool</dt><dd>{{ interaction.tool ?? '—' }}</dd>
                        <dt class="text-gray-500">Model</dt><dd>{{ interaction.model ?? '—' }}</dd>
                        <dt class="text-gray-500">Task</dt><dd>{{ interaction.task_id ?? 'untagged' }}</dd>
                        <dt class="text-gray-500">Branch</dt><dd>{{ interaction.branch ?? '—' }}</dd>
                        <dt class="text-gray-500">Tokens</dt>
                        <dd>{{ interaction.prompt_tokens }} in / {{ interaction.response_tokens }} out</dd>
                        <dt class="text-gray-500">Duration</dt><dd>{{ interaction.duration_ms }} ms</dd>
                        <dt class="text-gray-500">Kind</dt>
                        <dd>{{ interaction.automated ? 'automated follow-up' : 'human prompt' }}</dd>
                        <dt class="text-gray-500">Masked</dt>
                        <dd>{{ interaction.redacted?.length ? interaction.redacted.join(', ') : 'nothing' }}</dd>
                    </dl>
                </div>

                <div v-if="score" class="rounded-lg bg-white p-4 shadow">
                    <h3 class="mb-2 font-semibold">
                        Prompt quality: {{ score.score }}
                        <span class="text-sm font-normal text-gray-500">rubric v{{ score.rubric_version }}</span>
                    </h3>
                    <ul class="divide-y text-sm">
                        <li v-for="(dimension, name) in score.dimensions" :key="name" class="flex gap-3 py-2">
                            <span class="w-10 text-right font-semibold">{{ dimension.score }}</span>
                            <span>
                                <span class="font-medium">{{ String(name).replaceAll('_', ' ') }}</span>
                                <span class="block text-gray-600">{{ dimension.reason }}</span>
                            </span>
                        </li>
                    </ul>
                </div>

                <div v-if="canViewRaw" class="space-y-2">
                    <label class="block text-sm text-gray-600" for="reason">
                        Reason for opening — required for someone else's prompt, recorded in the audit log
                    </label>
                    <input id="reason" v-model="reason" type="text" placeholder="e.g. support investigation"
                           class="w-full rounded border-gray-300 text-sm shadow-sm" />
                    <p v-if="errors.reason" class="text-sm text-red-600">{{ errors.reason }}</p>
                    <div>
                        <Link :href="rawUrl"
                              class="inline-block rounded bg-gray-800 px-4 py-2 text-sm text-white">
                            Open the prompt text
                        </Link>
                        <p class="mt-2 text-xs text-gray-500">Opening it is recorded in the audit log.</p>
                    </div>
                </div>
                <p v-else class="text-sm text-gray-500">
                    You do not have permission to read the prompt text.
                </p>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
