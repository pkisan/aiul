<script setup>
// Team Pulse, the manager's home: is AI helping work get done, can we trust
// the numbers, and is anyone stuck? People are listed A-Z, never ranked.
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { ago, toolName } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';
import { computed } from 'vue';
import InsightTabs from '../../Components/InsightTabs.vue';
import Kpi from '../../Components/Kpi.vue';
import PeriodPicker from '../../Components/PeriodPicker.vue';
import Spark from '../../Components/Spark.vue';
import { usd } from '../../pm.js';

const props = defineProps({ kpis: Object, people: Array, attention: Array, period: Object, periods: Array, days: Array });

const k = computed(() => props.kpis);
const pct = (v) => (v === null ? '—' : `${v}%`);
const inPeriod = computed(() => (props.period.key === 'sprint' ? 'this sprint' : props.period.label.toLowerCase()));

const takeaways = computed(() => ({
    assisted: k.value.assisted.done
        ? `${k.value.assisted.ai} of ${k.value.assisted.done} tasks finished ${inPeriod.value} used AI${k.value.assisted.tool ? `; ${toolName(k.value.assisted.tool)} on ${k.value.assisted.toolTasks}` : ''}.`
        : `No tasks finished ${inPeriod.value} yet.`,
    prompts: k.value.prompts.perTask === null ? 'No AI-assisted task finished yet.' : `Across ${k.value.prompts.tasks} AI-assisted ${k.value.prompts.tasks === 1 ? 'task' : 'tasks'} that finished.`,
    cost: k.value.cost.tokens
        ? `Estimate at API list prices${k.value.cost.pricedPct < 100 ? `; ${k.value.cost.pricedPct}% of tokens have a price` : ''}.`
        : 'No AI use in this period.',
    linked: k.value.linked.total
        ? `${k.value.linked.linked} of ${k.value.linked.total} AI sessions are tied to a task.${k.value.linked.waiting ? ` ${k.value.linked.waiting} wait in the inbox.` : ''}`
        : 'No AI sessions in this period.',
}));

const quiet = computed(() => props.people.filter((p) => !p.sessions && !p.inProgress).length);
</script>

<template>
    <Head title="Team Pulse" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-6xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <div class="flex flex-wrap items-end gap-3">
                <div class="me-auto">
                    <InsightTabs :period="period" class="mb-3" />
                    <h1 class="text-xl font-semibold text-gray-900">Team Pulse</h1>
                    <p class="mt-1 text-sm text-gray-500">Is AI helping the work get done, and is anyone stuck?</p>
                </div>
                <PeriodPicker :period="period" :periods="periods" />
            </div>

            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                <Kpi label="Done tasks that used AI" :value="pct(k.assisted.pct)" :takeaway="takeaways.assisted" />
                <Kpi label="Prompts per AI-assisted task" :value="k.prompts.perTask" :takeaway="takeaways.prompts" />
                <Kpi label="AI cost" :value="usd(k.cost.usd)" :takeaway="takeaways.cost" :href="route('pm.tools', { period: period.key })">
                    <template #link>By tool and model →</template>
                </Kpi>
                <Kpi label="AI work linked to tasks" :value="pct(k.linked.pct)" :takeaway="takeaways.linked" />
            </div>

            <Panel title="Needs attention" :subtitle="`Many prompts in the last ${attention[0]?.hours ?? 48} hours on a task that has not moved: someone may be stuck.`">
                <p v-if="!attention.length" class="px-5 py-6 text-sm text-gray-500">Nothing looks stuck.</p>
                <ul v-else class="divide-y divide-gray-100">
                    <li v-for="a in attention" :key="a.id" class="flex flex-wrap items-baseline justify-between gap-2 px-5 py-3 text-sm">
                        <div class="min-w-0">
                            <Link :href="route('pm.tasks.show', a.id)" class="font-medium text-gray-900 hover:underline">
                                <span class="font-mono text-xs text-gray-500">{{ a.key }}</span> {{ a.title }}
                            </Link>
                            <p class="text-xs text-gray-500">
                                {{ a.assignee ?? 'Unassigned' }} · {{ a.prompts }} prompts, still in progress<template v-if="a.since"> (started {{ ago(a.since) }})</template>. A quick check-in may help.
                            </p>
                        </div>
                        <Link :href="route('pm.tasks.show', a.id)" class="shrink-0 text-xs font-medium text-indigo-600 hover:underline dark:text-indigo-400">See the trail →</Link>
                    </li>
                </ul>
            </Panel>

            <Panel title="People" :subtitle="`A-Z. Open a name for that person's tasks and tools. ${quiet ? `${quiet} with no AI sessions and nothing in progress.` : 'Everyone has something going.'}`">
                <div class="overflow-x-auto">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2.5">Person</th>
                                <th class="px-3 py-2.5">Main AI tool</th>
                                <th class="px-3 py-2.5 text-right">In progress</th>
                                <th class="px-3 py-2.5 text-right">AI sessions</th>
                                <th class="px-5 py-2.5">By day</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-for="p in people" :key="p.id">
                                <td class="px-5 py-2.5">
                                    <Link :href="route('pm.people.show', { user: p.id, period: period.key })" class="font-medium text-indigo-600 hover:underline dark:text-indigo-400">{{ p.name }}</Link>
                                </td>
                                <td class="px-3 py-2.5 text-gray-600">{{ p.tool ? toolName(p.tool) : '—' }}</td>
                                <td class="px-3 py-2.5 text-right tabular-nums text-gray-700">{{ p.inProgress }}</td>
                                <td class="px-3 py-2.5 text-right tabular-nums text-gray-700">{{ p.sessions }}</td>
                                <td class="px-5 py-2.5"><Spark :values="p.spark" :days="days" :label="p.name" /></td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </Panel>
        </div>
    </AuthenticatedLayout>
</template>
