<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { Head } from '@inertiajs/vue3';

defineProps({ views: Array });
</script>

<template>
    <Head title="Audit log" />

    <AuthenticatedLayout>
        <template #header>
            <h2 class="text-xl font-semibold leading-tight text-gray-800">Audit log</h2>
        </template>

        <div class="py-8">
            <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
                <p class="mb-4 text-sm text-gray-600">
                    Every time someone opens actual prompt text, it is recorded here. Nobody can
                    read a colleague's prompts without appearing in this list.
                </p>

                <div class="overflow-hidden rounded-lg bg-white shadow">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-gray-600">
                            <tr>
                                <th class="px-4 py-2">When</th>
                                <th class="px-4 py-2">Who looked</th>
                                <th class="px-4 py-2">Whose prompt</th>
                                <th class="px-4 py-2">Reason</th>
                                <th class="px-4 py-2">From</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y">
                            <tr v-for="row in views" :key="row.id">
                                <td class="px-4 py-2 text-gray-500">{{ row.at }}</td>
                                <td class="px-4 py-2 font-medium">{{ row.actor }}</td>
                                <td class="px-4 py-2">{{ row.subject }}</td>
                                <td class="px-4 py-2">{{ row.reason }}</td>
                                <td class="px-4 py-2 text-gray-400">{{ row.ip }}</td>
                            </tr>
                            <tr v-if="!views.length">
                                <td colspan="5" class="px-4 py-6 text-center text-gray-500">
                                    Nobody has opened raw prompt text.
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
