<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { Head } from '@inertiajs/vue3';

defineProps({
    interaction: Object,
    prompt: String,
    answer: String,
    promptState: String,
    answerState: String,
    auditNotice: String,
});
</script>

<template>
    <Head title="Prompt text" />

    <AuthenticatedLayout>
        <template #header>
            <h2 class="text-xl font-semibold leading-tight text-gray-800">Prompt text</h2>
        </template>

        <div class="py-8">
            <div class="mx-auto max-w-4xl space-y-4 px-4 sm:px-6 lg:px-8">
                <!-- The person reading is told, plainly, that this was recorded. -->
                <div class="rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900">
                    {{ auditNotice }}
                </div>

                <div class="rounded-lg bg-white p-4 text-sm shadow">
                    <div class="mb-2 text-gray-500">
                        {{ interaction.tool }} · {{ interaction.model }} ·
                        {{ interaction.task_id ?? 'untagged' }} · {{ interaction.occurred_at }}
                    </div>
                    <div v-if="interaction.redacted?.length" class="text-xs text-gray-500">
                        Masked before storage: {{ interaction.redacted.join(', ') }}
                    </div>
                </div>

                <div class="rounded-lg bg-white p-4 shadow">
                    <h3 class="mb-2 font-semibold">Prompt</h3>
                    <pre v-if="promptState === 'present'" class="whitespace-pre-wrap break-words text-sm">{{ prompt }}</pre>
                    <p v-else-if="promptState === 'purged'" class="text-sm text-gray-500">This body has been purged by the retention policy.</p>
                    <p v-else class="text-sm text-gray-500">No prompt text was captured in this exchange.</p>
                </div>

                <div class="rounded-lg bg-white p-4 shadow">
                    <h3 class="mb-2 font-semibold">Answer</h3>
                    <pre v-if="answerState === 'present'" class="whitespace-pre-wrap break-words text-sm">{{ answer }}</pre>
                    <p v-else-if="answerState === 'purged'" class="text-sm text-gray-500">This body has been purged by the retention policy.</p>
                    <p v-else class="text-sm text-gray-500">No answer text was captured in this exchange.</p>
                </div>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
