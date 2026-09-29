<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Pagination from '@/Components/Usage/Pagination.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { count, toolName, when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';

defineProps({ repo: String, name: String, interactions: Object, canViewRaw: Boolean });

const kinds = { human: 'Prompt', agent: 'Agent step', utility: "Tool's own call" };
</script>

<template>
    <Head :title="name ?? 'Outside a project'" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-6xl space-y-5 px-4 py-6 sm:px-6 lg:px-8">
            <div>
                <Link :href="route('usage.activity')" class="text-sm text-gray-500 hover:text-gray-900">← Activity</Link>
                <h1 class="mt-1 text-2xl font-semibold tracking-tight text-gray-900">{{ name ?? 'Outside a project' }}</h1>
                <p v-if="repo" class="mt-0.5 truncate font-mono text-xs text-gray-400">{{ repo }}</p>
            </div>

            <section class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
                <div class="overflow-x-auto">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2.5">When</th>
                                <th class="px-3 py-2.5">Person</th>
                                <th class="px-3 py-2.5">Tool</th>
                                <th class="px-3 py-2.5">Branch</th>
                                <th class="px-3 py-2.5">Kind</th>
                                <th class="px-5 py-2.5 text-right">Size</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-for="i in interactions.data" :key="i.id" class="hover:bg-gray-50">
                                <td class="whitespace-nowrap px-5 py-2.5">
                                    <Link :href="route('usage.session', i.session_id) + '#i-' + i.id" class="text-gray-900 hover:underline">
                                        {{ when(i.occurred_at) }}
                                    </Link>
                                </td>
                                <td class="px-3 py-2.5 text-gray-700">{{ i.person ?? 'Unassigned device' }}</td>
                                <td class="px-3 py-2.5"><Tag v-if="i.tool" :label="toolName(i.tool)" tone="blue" /></td>
                                <td class="px-3 py-2.5 text-gray-600">{{ i.branch ?? '—' }}</td>
                                <td class="px-3 py-2.5 text-gray-500">{{ kinds[i.kind] ?? i.kind }}</td>
                                <td class="whitespace-nowrap px-5 py-2.5 text-right text-xs tabular-nums text-gray-400">
                                    {{ count(i.prompt_chars) }} in / {{ count(i.answer_chars) }} out
                                </td>
                            </tr>
                            <tr v-if="!interactions.data.length">
                                <td colspan="6" class="px-5 py-12 text-center text-gray-500">Nothing captured for this project in the period.</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
                <Pagination :page="interactions" noun="interactions" />
            </section>
        </div>
    </AuthenticatedLayout>
</template>
