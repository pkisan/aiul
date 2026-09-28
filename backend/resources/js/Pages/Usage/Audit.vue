<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Pagination from '@/Components/Usage/Pagination.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';

defineProps({ views: Object });

const kinds = {
    raw_view: { label: 'Opened a prompt', tone: 'amber' },
    session_view: { label: 'Opened a session', tone: 'blue' },
    list_view: { label: 'Saw previews', tone: 'gray' },
};
</script>

<template>
    <Head title="Audit log" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-6xl space-y-5 px-4 py-6 sm:px-6 lg:px-8">
            <div>
                <h1 class="text-2xl font-semibold tracking-tight text-gray-900">Audit log</h1>
                <p class="mt-1 text-sm text-gray-500">
                    Every time someone sees actual prompt text it is recorded here. Nobody can read a colleague's
                    prompts without appearing in this list.
                </p>
            </div>

            <section class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
                <div class="overflow-x-auto">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2.5">When</th>
                                <th class="px-3 py-2.5">Who looked</th>
                                <th class="px-3 py-2.5">What</th>
                                <th class="px-3 py-2.5">Whose words</th>
                                <th class="px-3 py-2.5">Reason</th>
                                <th class="px-5 py-2.5">From</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-for="row in views.data" :key="row.id" class="hover:bg-gray-50">
                                <td class="whitespace-nowrap px-5 py-2.5 text-gray-500">{{ when(row.at) }}</td>
                                <td class="px-3 py-2.5 font-medium text-gray-900">{{ row.actor }}</td>
                                <td class="px-3 py-2.5">
                                    <Tag :label="kinds[row.kind]?.label ?? row.kind" :tone="kinds[row.kind]?.tone" />
                                </td>
                                <td class="px-3 py-2.5 text-gray-700">{{ row.subject }}</td>
                                <td class="px-3 py-2.5 text-gray-700">
                                    <Link v-if="row.interaction_id" :href="route('usage.show', row.interaction_id)" class="hover:underline">{{ row.reason }}</Link>
                                    <template v-else>{{ row.reason }}</template>
                                </td>
                                <td class="px-5 py-2.5 font-mono text-xs text-gray-400">{{ row.ip }}</td>
                            </tr>
                            <tr v-if="!views.data.length">
                                <td colspan="6" class="px-5 py-12 text-center text-gray-500">Nobody has seen prompt text yet.</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
                <Pagination :page="views" noun="records" />
            </section>
        </div>
    </AuthenticatedLayout>
</template>
