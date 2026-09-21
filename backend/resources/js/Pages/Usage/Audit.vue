<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { Head, Link } from '@inertiajs/vue3';

defineProps({ views: Array });
</script>

<template>
    <Head title="Audit log" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">Audit log</h2>
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-5xl space-y-4 px-4 sm:px-6 lg:px-8">
                <p class="text-sm leading-relaxed text-gray-600">
                    Every time someone opens actual prompt text it is recorded here. Nobody can read a
                    colleague's prompts without appearing in this list.
                </p>

                <Panel :title="`${views.length} reads`">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2 text-left font-medium">When</th>
                                <th class="px-3 py-2 text-left font-medium">Who looked</th>
                                <th class="px-3 py-2 text-left font-medium">Whose prompt</th>
                                <th class="px-3 py-2 text-left font-medium">Reason</th>
                                <th class="px-5 py-2 text-left font-medium">From</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-for="row in views" :key="row.id" class="hover:bg-gray-50">
                                <td class="px-5 py-2 text-gray-500">{{ row.at }}</td>
                                <td class="px-3 py-2 font-medium text-gray-900">{{ row.actor }}</td>
                                <td class="px-3 py-2 text-gray-700">{{ row.subject }}</td>
                                <td class="px-3 py-2 text-gray-700">{{ row.reason }}</td>
                                <td class="px-5 py-2 text-gray-400">{{ row.ip }}</td>
                            </tr>
                            <tr v-if="!views.length">
                                <td colspan="5" class="px-5 py-10 text-center text-gray-500">
                                    Nobody has opened raw prompt text.
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </Panel>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
