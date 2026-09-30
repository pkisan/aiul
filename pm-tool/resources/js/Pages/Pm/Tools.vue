<script setup>
// Tools & cost: which AI tools and models the team uses, and what finished
// work costs. Cost is an estimate at API list prices (pm-tool/config/pm.php).
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { count, toolName } from '@/Components/Usage/format';
import { Head } from '@inertiajs/vue3';
import { computed } from 'vue';
import DayBars from '../../Components/DayBars.vue';
import InsightTabs from '../../Components/InsightTabs.vue';
import Kpi from '../../Components/Kpi.vue';
import PeriodPicker from '../../Components/PeriodPicker.vue';
import { usd } from '../../pm.js';

const props = defineProps({ kpis: Object, rows: Array, unpriced: Array, days: Array, period: Object, periods: Array });

// Cost share by tool, for the takeaway sentence.
const byTool = computed(() => {
    const m = {};
    props.rows.forEach((r) => (m[r.tool] = (m[r.tool] ?? 0) + (r.usd ?? 0)));
    return Object.entries(m).sort((a, b) => b[1] - a[1]);
});
const topShare = computed(() => {
    const total = props.kpis.cost.usd;
    const [tool, amount] = byTool.value[0] ?? [];
    return total > 0 && tool ? `${toolName(tool)} is ${Math.round((100 * amount) / total)}% of the priced cost.` : 'Nothing priced in this period.';
});
const bars = computed(() => props.days.map((d) => ({ day: d.day, value: d.usd })));
</script>

<template>
    <Head title="Tools & cost" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <div class="flex flex-wrap items-end gap-3">
                <div class="me-auto">
                    <InsightTabs :period="period" class="mb-3" />
                    <h1 class="text-xl font-semibold text-gray-900">Tools &amp; cost</h1>
                    <p class="mt-1 text-sm text-gray-500">Estimated at API list prices. Chat subscriptions are not billed per token, so for them this is “what it would cost on the API”.</p>
                </div>
                <PeriodPicker :period="period" :periods="periods" />
            </div>

            <div class="grid gap-4 sm:grid-cols-3">
                <Kpi label="AI cost" :value="usd(kpis.cost.usd)" :takeaway="topShare" />
                <Kpi label="Cost per AI-assisted done task" :value="usd(kpis.perTask)" :takeaway="kpis.aiTasks ? `Across ${kpis.aiTasks} finished ${kpis.aiTasks === 1 ? 'task' : 'tasks'}.` : 'No AI-assisted task finished yet.'" />
                <Kpi label="Tokens" :value="count(kpis.cost.tokens)" :takeaway="kpis.cost.pricedPct === null || kpis.cost.pricedPct === 100 ? 'All tokens have a price.' : `${kpis.cost.pricedPct}% have a price; the rest count as $0.`" />
            </div>

            <Panel title="Cost by day" :subtitle="`${usd(kpis.cost.usd)} ${period.key === 'sprint' ? 'this sprint' : `in the ${period.label.toLowerCase()}`}. Hover a day for its cost.`">
                <div class="px-5 py-4"><DayBars :days="bars" :format="usd" label="Estimated AI cost per day" /></div>
            </Panel>

            <Panel title="By tool and model" :subtitle="unpriced.length ? `No price set for: ${unpriced.join(', ')}. Add prices in pm-tool/config/pm.php.` : 'Every model has a price.'">
                <div class="overflow-x-auto">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2.5">Tool</th>
                                <th class="px-3 py-2.5">Model</th>
                                <th class="px-3 py-2.5 text-right">Sessions</th>
                                <th class="px-3 py-2.5 text-right">Prompts</th>
                                <th class="px-3 py-2.5 text-right">Tokens</th>
                                <th class="px-5 py-2.5 text-right">Cost</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-if="!rows.length"><td colspan="6" class="px-5 py-6 text-center text-gray-500">No AI use in this period.</td></tr>
                            <tr v-for="r in rows" :key="`${r.tool}|${r.model}`">
                                <td class="px-5 py-2.5 text-gray-900">{{ toolName(r.tool) }}</td>
                                <td class="px-3 py-2.5 font-mono text-xs text-gray-600">{{ r.model || '—' }}</td>
                                <td class="px-3 py-2.5 text-right tabular-nums text-gray-700">{{ count(r.sessions) }}</td>
                                <td class="px-3 py-2.5 text-right tabular-nums text-gray-700">{{ count(r.prompts) }}</td>
                                <td class="px-3 py-2.5 text-right tabular-nums text-gray-700">{{ count(r.tokens) }}</td>
                                <td class="px-5 py-2.5 text-right tabular-nums" :class="r.usd === null ? 'text-gray-400' : 'text-gray-900'">{{ r.usd === null ? 'no price' : usd(r.usd) }}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </Panel>
        </div>
    </AuthenticatedLayout>
</template>
