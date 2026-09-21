<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { Head, Link } from '@inertiajs/vue3';

defineProps({ task: String, untagged: Boolean, interactions: Array });
</script>

<template>
    <Head :title="untagged ? 'Untagged work' : `Task ${task}`" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-4">
                <Link :href="route('usage.index')" class="text-sm text-gray-600 underline">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">
                    {{ untagged ? 'Untagged (no branch ticket)' : task }}
                </h2>
            </div>
        </template>

        <div class="py-8">
            <div class="mx-auto max-w-5xl px-4 sm:px-6 lg:px-8">
                <div class="overflow-hidden rounded-lg bg-white shadow">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-gray-600">
                            <tr>
                                <th class="px-4 py-2">When</th>
                                <th class="px-4 py-2">Tool</th>
                                <th class="px-4 py-2">Model</th>
                                <th class="px-4 py-2">Kind</th>
                                <th class="px-4 py-2 text-right">Score</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y">
                            <tr v-for="i in interactions" :key="i.id" class="hover:bg-gray-50">
                                <td class="px-4 py-2">
                                    <Link :href="route('usage.show', i.id)" class="underline">
                                        {{ new Date(i.occurred_at).toLocaleString() }}
                                    </Link>
                                </td>
                                <td class="px-4 py-2">{{ i.tool ?? '—' }}</td>
                                <td class="px-4 py-2 text-gray-500">{{ i.model ?? '—' }}</td>
                                <td class="px-4 py-2 text-gray-500">
                                    {{ i.automated ? 'follow-up' : 'prompt' }}
                                </td>
                                <td class="px-4 py-2 text-right font-medium">{{ i.score ?? '—' }}</td>
                            </tr>
                            <tr v-if="!interactions.length">
                                <td colspan="5" class="px-4 py-6 text-center text-gray-500">
                                    Nothing captured for this task in the period.
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
