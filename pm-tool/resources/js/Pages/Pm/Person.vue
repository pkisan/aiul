<script setup>
// One person's work and where AI helped. Framed as support: what they are
// on, which tools they reach for, how much of it is tied to tasks.
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { duration, toolName } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';
import { computed } from 'vue';
import DayBars from '../../Components/DayBars.vue';
import Kpi from '../../Components/Kpi.vue';
import PeriodPicker from '../../Components/PeriodPicker.vue';
import { statusLabels } from '../../pm.js';

const props = defineProps({ person: Object, isMe: Boolean, kpis: Object, split: Object, tools: Array, days: Array, tasks: Array, period: Object, periods: Array });

const inPeriod = computed(() => (props.period.key === 'sprint' ? 'this sprint' : props.period.label.toLowerCase()));
const top = computed(() => props.tools[0]);
const toolTotal = computed(() => props.tools.reduce((a, t) => a + t.n, 0));
const bars = computed(() => props.days.map((d) => ({ day: d.day, value: d.n })));
const busiest = computed(() => Math.max(0, ...props.days.map((d) => d.n)));
const splitRows = computed(() => [
    ['Linked to a task', props.split.linked],
    ['Suggested, not confirmed', props.split.suggested],
    ['Not task work', props.split.notWork],
    ['Not linked', props.split.unlinked],
]);
</script>

<template>
    <Head :title="isMe ? 'My work' : person.name" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <div class="flex flex-wrap items-end gap-3">
                <div class="me-auto">
                    <Link v-if="!isMe" :href="route('pm.pulse', { period: period.key })" class="text-sm text-gray-500 hover:underline">← Team Pulse</Link>
                    <h1 class="text-xl font-semibold text-gray-900">{{ isMe ? 'My work' : person.name }}</h1>
                    <p class="mt-1 text-sm text-gray-500">
                        What {{ isMe ? 'you are' : `${person.name} is` }} working on and where AI helps. For support, not for ranking.
                    </p>
                </div>
                <PeriodPicker :period="period" :periods="periods" />
            </div>

            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                <Kpi label="Tasks done" :value="kpis.done" :takeaway="`Finished ${inPeriod}.`" />
                <Kpi label="AI sessions" :value="kpis.sessions" :takeaway="top ? `Mostly ${toolName(top.tool)} (${top.n} of ${toolTotal}).` : 'No AI use in this period.'" />
                <Kpi label="Linked to tasks" :value="kpis.linkedPct === null ? '—' : `${kpis.linkedPct}%`" :takeaway="split.suggested || split.unlinked ? `${split.suggested + split.unlinked} sessions wait in the inbox.` : 'Everything is sorted.'" />
                <Kpi label="AI time" :value="duration(kpis.seconds)" takeaway="Sum of AI session lengths." />
            </div>

            <Panel title="Tasks" :subtitle="`Open now, plus done ${inPeriod}.`">
                <p v-if="!tasks.length" class="px-5 py-6 text-sm text-gray-500">No tasks assigned.</p>
                <ul v-else class="divide-y divide-gray-100 text-sm">
                    <li v-for="t in tasks" :key="t.id" class="flex flex-wrap items-baseline justify-between gap-2 px-5 py-2.5">
                        <Link :href="route('pm.tasks.show', t.id)" class="min-w-0 text-gray-900 hover:underline">
                            <span class="font-mono text-xs text-gray-500">{{ t.key }}</span> {{ t.title }}
                        </Link>
                        <span class="text-xs text-gray-500">{{ statusLabels[t.status] }}<template v-if="t.ai_sessions"> · {{ t.ai_sessions }} AI {{ t.ai_sessions === 1 ? 'session' : 'sessions' }}</template></span>
                    </li>
                </ul>
            </Panel>

            <div class="grid gap-6 lg:grid-cols-2">
                <Panel title="AI sessions by day" :subtitle="busiest ? `Busiest day: ${busiest} ${busiest === 1 ? 'session' : 'sessions'}.` : 'No sessions in this period.'">
                    <div class="px-5 py-4"><DayBars :days="bars" :label="`AI sessions per day for ${person.name}`" /></div>
                </Panel>

                <Panel title="Tools" :subtitle="top ? `${toolName(top.tool)} is ${Math.round((100 * top.n) / toolTotal)}% of sessions.` : 'No sessions in this period.'">
                    <ul class="space-y-2 px-5 py-4 text-sm">
                        <li v-for="t in tools" :key="t.tool" class="grid grid-cols-[8rem_1fr_2.5rem] items-center gap-3">
                            <span class="truncate text-gray-700">{{ toolName(t.tool) }}</span>
                            <span class="h-2 rounded-full bg-gray-100"><span class="block h-2 rounded-full bg-indigo-600 dark:bg-indigo-400" :style="{ width: `${(100 * t.n) / toolTotal}%` }"></span></span>
                            <span class="text-right tabular-nums text-gray-600">{{ t.n }}</span>
                        </li>
                    </ul>
                    <div class="border-t border-gray-100 px-5 py-3">
                        <dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
                            <template v-for="[label, n] in splitRows" :key="label">
                                <dt class="text-gray-500">{{ label }}</dt>
                                <dd class="text-right tabular-nums text-gray-700">{{ n }}</dd>
                            </template>
                        </dl>
                    </div>
                </Panel>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
