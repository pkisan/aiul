<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Pagination from '@/Components/Usage/Pagination.vue';
import Panel from '@/Components/Usage/Panel.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { ago, toolName, when } from '@/Components/Usage/format';
import { Head, router } from '@inertiajs/vue3';

defineProps({ deletions: Object, trashDays: Number, canRead: Boolean });

const restore = (d) => router.post(route('usage.deleted.restore', d.deletion_id), {}, { preserveScroll: true });
const purge = (d) => {
    if (confirm('Remove this for good? The text is erased now and cannot be restored.')) {
        router.delete(route('usage.deleted.destroy', d.deletion_id), { preserveScroll: true });
    }
};
</script>

<template>
    <Head title="Deleted prompts" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-5xl space-y-5 px-4 py-6 sm:px-6 lg:px-8">
            <div>
                <h1 class="text-2xl font-semibold tracking-tight text-gray-900">Deleted prompts</h1>
                <p class="mt-1 text-sm text-gray-500">
                    Hidden from every page and count. Restore puts a prompt back with its answer; after
                    {{ trashDays }} days it is removed for good.
                </p>
            </div>

            <Panel>
                <p v-if="!deletions.data.length" class="px-5 py-10 text-center text-sm text-gray-400">Nothing deleted.</p>
                <ul v-else class="divide-y divide-gray-100">
                    <li v-for="d in deletions.data" :key="d.deletion_id" class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-start">
                        <div class="min-w-0 flex-1">
                            <div class="flex flex-wrap items-center gap-2 text-sm">
                                <span class="font-medium text-gray-900">{{ d.person ?? 'Unassigned device' }}</span>
                                <Tag v-if="d.tool" :label="toolName(d.tool)" tone="blue" />
                                <span class="text-gray-400" :title="when(d.occurred_at)">{{ ago(d.occurred_at) }}</span>
                            </div>
                            <p v-if="canRead" class="mt-1 break-words text-sm text-gray-700">{{ d.prompt || 'No text captured.' }}</p>
                            <p class="mt-1 text-xs text-gray-400">
                                {{ d.rows }} {{ d.rows === 1 ? 'row' : 'rows' }} · deleted {{ ago(d.deleted_at) }}
                                <template v-if="d.deleted_by"> by {{ d.deleted_by }}</template>
                                · removed for good {{ when(d.purge_at) }}
                            </p>
                        </div>
                        <div class="flex shrink-0 gap-4 text-sm">
                            <button type="button" class="font-medium text-indigo-600 hover:underline dark:text-indigo-400" @click="restore(d)">Restore</button>
                            <button type="button" class="text-gray-500 hover:text-rose-600 hover:underline" @click="purge(d)">Delete permanently</button>
                        </div>
                    </li>
                </ul>
                <Pagination :page="deletions" noun="deletions" />
            </Panel>
        </div>
    </AuthenticatedLayout>
</template>
