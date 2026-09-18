<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { Head, Link } from '@inertiajs/vue3';

defineProps({
    totals: Object,
    perTask: Array,
    perPerson: Array,
    weakest: Array,
    aiTimeDefinition: String,
    canViewRaw: Boolean,
});

// Seconds read badly as a raw number on a dashboard.
const hours = (seconds) => {
    if (!seconds) return '—';
    const h = Math.floor(seconds / 3600);
    const m = Math.round((seconds % 3600) / 60);
    return h ? `${h}h ${m}m` : `${m}m`;
};

const scoreClass = (score) => {
    if (score === null || score === undefined) return 'text-gray-400';
    if (score >= 75) return 'text-green-700';
    if (score >= 50) return 'text-amber-600';
    return 'text-red-600';
};
</script>

<template>
    <Head title="AI usage" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center justify-between">
                <h2 class="text-xl font-semibold leading-tight text-gray-800">AI usage</h2>
                <Link :href="route('usage.audit')" class="text-sm text-gray-600 underline">Audit log</Link>
            </div>
        </template>

        <div class="py-8">
            <div class="mx-auto max-w-7xl space-y-6 px-4 sm:px-6 lg:px-8">
                <!-- Totals -->
                <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
                    <div class="rounded-lg bg-white p-4 shadow">
                        <div class="text-sm text-gray-500">Interactions</div>
                        <div class="text-2xl font-semibold">{{ totals.interactions }}</div>
                        <div class="text-xs text-gray-400">last {{ totals.days }} days</div>
                    </div>
                    <div class="rounded-lg bg-white p-4 shadow">
                        <div class="text-sm text-gray-500">Human prompts</div>
                        <div class="text-2xl font-semibold">{{ totals.human_prompts }}</div>
                        <div class="text-xs text-gray-400">the rest are automated follow-ups</div>
                    </div>
                    <div class="rounded-lg bg-white p-4 shadow">
                        <div class="text-sm text-gray-500">Untagged</div>
                        <div class="text-2xl font-semibold">{{ totals.untagged }}</div>
                        <div class="text-xs text-gray-400">no task could be determined</div>
                    </div>
                    <div class="rounded-lg bg-white p-4 shadow">
                        <div class="text-sm text-gray-500">Tools in use</div>
                        <div class="text-2xl font-semibold">{{ totals.tools }}</div>
                    </div>
                </div>

                <!-- The definition is on the page on purpose: a metric people cannot
                     explain is a metric they will argue with. -->
                <div class="rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm text-blue-900">
                    <strong>How "AI time" is measured.</strong> {{ aiTimeDefinition }}
                </div>

                <!-- Per task -->
                <div class="overflow-hidden rounded-lg bg-white shadow">
                    <h3 class="border-b px-4 py-3 font-semibold">Per task</h3>
                    <div class="overflow-x-auto">
                        <table class="min-w-full text-sm">
                            <thead class="bg-gray-50 text-left text-gray-600">
                                <tr>
                                    <th class="px-4 py-2">Task</th>
                                    <th class="px-4 py-2 text-right">Prompts</th>
                                    <th class="px-4 py-2 text-right">Follow-ups</th>
                                    <th class="px-4 py-2 text-right">AI time</th>
                                    <th class="px-4 py-2 text-right">Tokens</th>
                                    <th class="px-4 py-2 text-right">Avg score</th>
                                </tr>
                            </thead>
                            <tbody class="divide-y">
                                <tr v-for="row in perTask" :key="row.task_id ?? 'untagged'"
                                    :class="row.untagged ? 'bg-amber-50' : ''">
                                    <td class="px-4 py-2 font-medium">
                                        <span v-if="row.untagged" class="text-amber-800">
                                            Untagged
                                            <span class="ml-1 text-xs font-normal text-amber-700">
                                                (no branch ticket)
                                            </span>
                                        </span>
                                        <span v-else>{{ row.task_id }}</span>
                                    </td>
                                    <td class="px-4 py-2 text-right">{{ row.human_prompts }}</td>
                                    <td class="px-4 py-2 text-right text-gray-500">{{ row.automated_followups }}</td>
                                    <td class="px-4 py-2 text-right">{{ hours(row.ai_seconds) }}</td>
                                    <td class="px-4 py-2 text-right text-gray-500">{{ row.tokens }}</td>
                                    <td class="px-4 py-2 text-right font-medium" :class="scoreClass(row.average_score)">
                                        {{ row.average_score ?? '—' }}
                                    </td>
                                </tr>
                                <tr v-if="!perTask.length">
                                    <td colspan="6" class="px-4 py-6 text-center text-gray-500">
                                        Nothing captured yet.
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <!-- Per person -->
                <div class="overflow-hidden rounded-lg bg-white shadow">
                    <h3 class="border-b px-4 py-3 font-semibold">Per person</h3>
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-gray-600">
                            <tr>
                                <th class="px-4 py-2">Person</th>
                                <th class="px-4 py-2 text-right">Interactions</th>
                                <th class="px-4 py-2 text-right">Tasks</th>
                                <th class="px-4 py-2 text-right">AI time</th>
                                <th class="px-4 py-2 text-right">Avg score</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y">
                            <tr v-for="row in perPerson" :key="row.user_id ?? 'none'">
                                <td class="px-4 py-2">{{ row.name }}</td>
                                <td class="px-4 py-2 text-right">{{ row.interactions }}</td>
                                <td class="px-4 py-2 text-right">{{ row.tasks }}</td>
                                <td class="px-4 py-2 text-right">{{ hours(row.ai_seconds) }}</td>
                                <td class="px-4 py-2 text-right font-medium" :class="scoreClass(row.average_score)">
                                    {{ row.average_score ?? '—' }}
                                </td>
                            </tr>
                            <tr v-if="!perPerson.length">
                                <td colspan="5" class="px-4 py-6 text-center text-gray-500">No people yet.</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <!-- Coaching, not ranking -->
                <div class="overflow-hidden rounded-lg bg-white shadow">
                    <h3 class="border-b px-4 py-3 font-semibold">
                        Where prompts are weakest
                        <span class="ml-2 text-sm font-normal text-gray-500">
                            across everyone — this is for coaching, not ranking
                        </span>
                    </h3>
                    <ul class="divide-y">
                        <li v-for="row in weakest" :key="row.dimension" class="flex items-start gap-4 px-4 py-3">
                            <span class="w-16 text-right font-semibold" :class="scoreClass(row.average)">
                                {{ row.average }}
                            </span>
                            <span class="flex-1">
                                <span class="font-medium">{{ row.dimension.replaceAll('_', ' ') }}</span>
                                <span class="block text-sm text-gray-600">{{ row.common_reason }}</span>
                            </span>
                            <span class="text-xs text-gray-400">{{ row.sample }} prompts</span>
                        </li>
                        <li v-if="!weakest.length" class="px-4 py-6 text-center text-gray-500">
                            No scored prompts yet.
                        </li>
                    </ul>
                </div>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
