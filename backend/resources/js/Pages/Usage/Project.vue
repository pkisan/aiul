<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { count, when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';

defineProps({ repo: String, name: String, interactions: Object, canViewRaw: Boolean });
</script>

<template>
    <Head :title="name ?? 'Unknown project'" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">{{ name ?? 'Unknown project' }}</h2>
                <span v-if="repo" class="truncate text-xs text-gray-400">{{ repo }}</span>
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-5xl px-4 sm:px-6 lg:px-8">
                <Panel :title="`${interactions.total} interactions`">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2 text-left font-medium">When</th>
                                <th class="px-3 py-2 text-left font-medium">Branch</th>
                                <th class="px-3 py-2 text-left font-medium">Tool</th>
                                <th class="px-3 py-2 text-left font-medium">Kind</th>
                                <th class="px-5 py-2 text-right font-medium">Score</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-for="i in interactions.data" :key="i.id" class="hover:bg-gray-50">
                                <td class="px-5 py-2">
                                    <Link :href="route('usage.show', i.id)" class="text-gray-900 hover:underline">
                                        {{ when(i.occurred_at) }}
                                    </Link>
                                    <span class="block text-xs text-gray-400 tabular-nums">
                                        {{ count(i.prompt_chars) }} in / {{ count(i.answer_chars) }} out chars
                                    </span>
                                </td>
                                <td class="px-3 py-2 text-gray-600">{{ i.branch ?? '—' }}</td>
                                <td class="px-3 py-2"><Tag v-if="i.tool" :label="i.tool" tone="blue" /></td>
                                <td class="px-3 py-2 text-gray-500">{{ i.automated ? 'follow-up' : 'prompt' }}</td>
                                <td class="px-5 py-2 text-right"><Score :value="i.score" /></td>
                            </tr>
                            <tr v-if="!interactions.data.length">
                                <td colspan="5" class="px-5 py-10 text-center text-gray-500">
                                    Nothing captured for this project in the period.
                                </td>
                            </tr>
                        </tbody>
                    </table>

                    <div
                        v-if="interactions.last_page > 1"
                        class="flex items-center justify-between border-t border-gray-100 px-5 py-3 text-sm"
                    >
                        <span class="text-gray-500">Page {{ interactions.current_page }} of {{ interactions.last_page }}</span>
                        <span class="flex gap-1">
                            <Link
                                v-for="link in interactions.links"
                                :key="link.label"
                                :href="link.url ?? ''"
                                :only="['interactions']"
                                :preserve-scroll="true"
                                v-html="link.label"
                                class="rounded-md px-2 py-1"
                                :class="{
                                    'bg-gray-900 text-white': link.active,
                                    'text-gray-600 hover:bg-gray-100': !link.active && link.url,
                                    'text-gray-300': !link.url,
                                }"
                            />
                        </span>
                    </div>
                </Panel>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
