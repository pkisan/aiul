<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { Head } from '@inertiajs/vue3';

defineProps({
    summary: Object,
    interactions: Array,
    whoLooked: Array,
    explanation: Array,
});
</script>

<template>
    <Head title="My data" />

    <AuthenticatedLayout>
        <template #header>
            <h2 class="text-xl font-semibold leading-tight text-gray-800">My data</h2>
        </template>

        <div class="py-8">
            <div class="mx-auto max-w-5xl space-y-6 px-4 sm:px-6 lg:px-8">
                <div class="rounded-lg bg-white p-4 shadow">
                    <h3 class="mb-2 font-semibold">What is captured</h3>
                    <ul class="list-disc space-y-1 pl-5 text-sm text-gray-700">
                        <li v-for="line in explanation" :key="line">{{ line }}</li>
                    </ul>
                </div>

                <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
                    <div class="rounded-lg bg-white p-4 shadow">
                        <div class="text-sm text-gray-500">Interactions captured</div>
                        <div class="text-2xl font-semibold">{{ summary.total }}</div>
                    </div>
                    <div class="rounded-lg bg-white p-4 shadow">
                        <div class="text-sm text-gray-500">Since</div>
                        <div class="text-lg font-semibold">{{ summary.first_seen ?? '—' }}</div>
                    </div>
                    <div class="rounded-lg bg-white p-4 shadow">
                        <div class="text-sm text-gray-500">My devices</div>
                        <div class="text-lg font-semibold">{{ summary.devices.length }}</div>
                        <div v-for="device in summary.devices" :key="device.id" class="text-xs text-gray-400">
                            {{ device.hostname }}
                        </div>
                    </div>
                </div>

                <!-- Who read this person's words. The point of the whole page. -->
                <div class="overflow-hidden rounded-lg bg-white shadow">
                    <h3 class="border-b px-4 py-3 font-semibold">Who has read my prompts</h3>
                    <table class="min-w-full text-sm">
                        <tbody class="divide-y">
                            <tr v-for="(row, index) in whoLooked" :key="index">
                                <td class="px-4 py-2 text-gray-500">{{ row.at }}</td>
                                <td class="px-4 py-2 font-medium">{{ row.actor }}</td>
                                <td class="px-4 py-2">{{ row.reason }}</td>
                            </tr>
                            <tr v-if="!whoLooked.length">
                                <td class="px-4 py-6 text-center text-gray-500">
                                    Nobody else has opened your prompt text.
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <div class="overflow-hidden rounded-lg bg-white shadow">
                    <h3 class="border-b px-4 py-3 font-semibold">My recent interactions</h3>
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-gray-600">
                            <tr>
                                <th class="px-4 py-2">When</th>
                                <th class="px-4 py-2">Tool</th>
                                <th class="px-4 py-2">Task</th>
                                <th class="px-4 py-2">Kind</th>
                                <th class="px-4 py-2">Masked</th>
                                <th class="px-4 py-2 text-right">Score</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y">
                            <tr v-for="row in interactions" :key="row.id">
                                <td class="px-4 py-2 text-gray-500">{{ row.occurred_at }}</td>
                                <td class="px-4 py-2">{{ row.tool ?? '—' }}</td>
                                <td class="px-4 py-2">{{ row.task_id ?? 'untagged' }}</td>
                                <td class="px-4 py-2 text-gray-500">
                                    {{ row.automated ? 'automated follow-up' : 'my prompt' }}
                                </td>
                                <td class="px-4 py-2 text-xs text-gray-500">
                                    {{ row.redacted?.length ? row.redacted.join(', ') : '—' }}
                                </td>
                                <td class="px-4 py-2 text-right">{{ row.score ?? '—' }}</td>
                            </tr>
                            <tr v-if="!interactions.length">
                                <td colspan="6" class="px-4 py-6 text-center text-gray-500">
                                    Nothing has been captured about you.
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
