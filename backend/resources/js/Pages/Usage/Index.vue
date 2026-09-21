<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import StatCard from '@/Components/Usage/StatCard.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { count, duration, when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';

defineProps({
    totals: Object,
    perTask: Array,
    perPerson: Array,
    weakest: Array,
    aiTimeDefinition: String,
    canViewRaw: Boolean,
    recent: Array,
    sessions: Array,
});
</script>

<template>
    <Head title="AI usage" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center justify-between">
                <h2 class="text-xl font-semibold leading-tight text-gray-800">AI usage</h2>
                <Link :href="route('usage.audit')" class="text-sm text-gray-500 hover:text-gray-900">Audit log</Link>
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-6xl space-y-6 px-4 sm:px-6 lg:px-8">
                <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
                    <StatCard label="Interactions" :value="count(totals.interactions)" :hint="`last ${totals.days} days`" />
                    <StatCard label="Human prompts" :value="count(totals.human_prompts)" hint="the rest are automated follow-ups" />
                    <StatCard label="Untagged" :value="count(totals.untagged)" hint="no task could be determined" />
                    <StatCard label="Tools in use" :value="count(totals.tools)" />
                </div>

                <!-- Sessions lead, because this is how the work actually happened:
                     one message to an agent produces a dozen interactions, and a
                     flat list of them buries the shape of a day. -->
                <Panel
                    title="Sessions"
                    subtitle="One stretch of work: same task, same device, no gap longer than the idle window"
                >
                    <div class="divide-y divide-gray-100">
                        <Link
                            v-for="s in sessions"
                            :key="s.id"
                            :href="route('usage.session', s.id)"
                            class="flex items-center gap-4 px-5 py-3 transition hover:bg-gray-50"
                        >
                            <div class="min-w-0 flex-1">
                                <div class="flex items-center gap-2">
                                    <span class="truncate text-sm font-medium text-gray-900">
                                        {{ s.task_id ?? 'Untagged' }}
                                    </span>
                                    <Tag v-if="s.tool" :label="s.tool" tone="blue" />
                                    <Tag v-if="s.branch" :label="s.branch" />
                                </div>
                                <div class="mt-0.5 truncate text-xs text-gray-500">
                                    {{ when(s.started_at) }}
                                    <template v-if="s.person"> · {{ s.person }}</template>
                                    <template v-if="s.repo"> · {{ s.repo }}</template>
                                </div>
                            </div>

                            <div class="hidden shrink-0 text-right sm:block">
                                <div class="text-sm tabular-nums text-gray-900">{{ s.human_prompts }}</div>
                                <div class="text-xs text-gray-400">prompts</div>
                            </div>
                            <div class="hidden shrink-0 text-right sm:block">
                                <div class="text-sm tabular-nums text-gray-900">{{ duration(s.seconds) }}</div>
                                <div class="text-xs text-gray-400">AI time</div>
                            </div>
                            <div class="shrink-0 text-right">
                                <Score :value="s.avg_score" />
                                <div class="text-xs text-gray-400">avg</div>
                            </div>
                        </Link>

                        <p v-if="!sessions.length" class="px-5 py-10 text-center text-sm text-gray-500">
                            Nothing captured yet.
                        </p>
                    </div>
                </Panel>

                <div class="grid gap-6 lg:grid-cols-2">
                    <Panel title="Per task" subtitle="Untagged work is shown, never hidden">
                        <table class="min-w-full text-sm">
                            <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                                <tr>
                                    <th class="px-5 py-2 text-left font-medium">Task</th>
                                    <th class="px-3 py-2 text-right font-medium">Prompts</th>
                                    <th class="px-3 py-2 text-right font-medium">AI time</th>
                                    <th class="px-5 py-2 text-right font-medium">Score</th>
                                </tr>
                            </thead>
                            <tbody class="divide-y divide-gray-100">
                                <tr
                                    v-for="row in perTask"
                                    :key="row.task_id ?? 'untagged'"
                                    class="hover:bg-gray-50"
                                >
                                    <td class="px-5 py-2">
                                        <Link
                                            :href="route('usage.task', row.untagged ? 'untagged' : row.task_id)"
                                            class="font-medium text-gray-900 hover:underline"
                                        >
                                            <span v-if="row.untagged" class="text-amber-700">Untagged</span>
                                            <span v-else>{{ row.task_id }}</span>
                                        </Link>
                                        <span class="block text-xs text-gray-400">
                                            {{ row.automated_followups }} follow-ups · {{ count(row.tokens) }} tokens
                                        </span>
                                    </td>
                                    <td class="px-3 py-2 text-right tabular-nums">{{ row.human_prompts }}</td>
                                    <td class="px-3 py-2 text-right tabular-nums text-gray-600">{{ duration(row.ai_seconds) }}</td>
                                    <td class="px-5 py-2 text-right"><Score :value="row.average_score" /></td>
                                </tr>
                                <tr v-if="!perTask.length">
                                    <td colspan="4" class="px-5 py-10 text-center text-gray-500">Nothing captured yet.</td>
                                </tr>
                            </tbody>
                        </table>
                    </Panel>

                    <Panel title="Per person">
                        <table class="min-w-full text-sm">
                            <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                                <tr>
                                    <th class="px-5 py-2 text-left font-medium">Person</th>
                                    <th class="px-3 py-2 text-right font-medium">Interactions</th>
                                    <th class="px-3 py-2 text-right font-medium">AI time</th>
                                    <th class="px-5 py-2 text-right font-medium">Score</th>
                                </tr>
                            </thead>
                            <tbody class="divide-y divide-gray-100">
                                <tr v-for="row in perPerson" :key="row.user_id ?? 'none'" class="hover:bg-gray-50">
                                    <td class="px-5 py-2">
                                        <span class="font-medium text-gray-900">{{ row.name }}</span>
                                        <span class="block text-xs text-gray-400">{{ row.tasks }} tasks</span>
                                    </td>
                                    <td class="px-3 py-2 text-right tabular-nums">{{ count(row.interactions) }}</td>
                                    <td class="px-3 py-2 text-right tabular-nums text-gray-600">{{ duration(row.ai_seconds) }}</td>
                                    <td class="px-5 py-2 text-right"><Score :value="row.average_score" /></td>
                                </tr>
                                <tr v-if="!perPerson.length">
                                    <td colspan="4" class="px-5 py-10 text-center text-gray-500">No people yet.</td>
                                </tr>
                            </tbody>
                        </table>
                    </Panel>
                </div>

                <Panel title="Where prompts are weakest" subtitle="Across everyone — for coaching, not ranking">
                    <ul class="divide-y divide-gray-100">
                        <li v-for="row in weakest" :key="row.dimension" class="flex items-start gap-4 px-5 py-3">
                            <Score :value="row.average" class="w-10 shrink-0 text-right" />
                            <div class="min-w-0 flex-1">
                                <div class="text-sm font-medium capitalize text-gray-900">
                                    {{ row.dimension.replaceAll('_', ' ') }}
                                </div>
                                <p class="text-sm text-gray-600">{{ row.common_reason }}</p>
                            </div>
                            <span class="shrink-0 text-xs text-gray-400">{{ row.sample }} prompts</span>
                        </li>
                        <li v-if="!weakest.length" class="px-5 py-10 text-center text-sm text-gray-500">
                            No scored prompts yet.
                        </li>
                    </ul>
                </Panel>

                <Panel title="Recent interactions" subtitle="The short path to a single prompt">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2 text-left font-medium">When</th>
                                <th class="px-3 py-2 text-left font-medium">Task</th>
                                <th class="px-3 py-2 text-left font-medium">Tool</th>
                                <th class="px-5 py-2 text-right font-medium">Score</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-for="i in recent" :key="i.id" class="hover:bg-gray-50">
                                <td class="px-5 py-2">
                                    <Link :href="route('usage.show', i.id)" class="text-gray-900 hover:underline">
                                        {{ when(i.occurred_at) }}
                                    </Link>
                                </td>
                                <td class="px-3 py-2 text-gray-600">{{ i.task_id ?? 'untagged' }}</td>
                                <td class="px-3 py-2"><Tag v-if="i.tool" :label="i.tool" /></td>
                                <td class="px-5 py-2 text-right"><Score :value="i.score" /></td>
                            </tr>
                            <tr v-if="!recent.length">
                                <td colspan="4" class="px-5 py-10 text-center text-gray-500">Nothing captured yet.</td>
                            </tr>
                        </tbody>
                    </table>
                </Panel>

                <p class="px-1 text-xs leading-relaxed text-gray-500">
                    <strong class="text-gray-700">How "AI time" is measured.</strong> {{ aiTimeDefinition }}
                </p>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
