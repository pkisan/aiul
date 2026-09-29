<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { ago, count, duration, initials, toolColor, toolName, when } from '@/Components/Usage/format';
import { Head, Link, router } from '@inertiajs/vue3';
import { computed, ref } from 'vue';

const props = defineProps({
    days: Number,
    summary: Object,
    previous: Object,
    enrolled: Number,
    attention: Object,
    activity: Object,
    trends: Object,
    perTool: Array,
    perProject: Array,
    team: Array,
    latest: Array,
    insights: Array,
    aiTimeDefinition: String,
});

const periods = [
    { value: 1, label: 'Today' },
    { value: 7, label: '7 days' },
    { value: 30, label: '30 days' },
    { value: 90, label: '90 days' },
    { value: 365, label: '1 year' },
];
const periodName = computed(() => (props.days === 1 ? 'last 24 hours' : `last ${props.days} days`));

const setDays = (days) => router.get(route('usage.index'), days === 7 ? {} : { days }, { preserveScroll: true });

// A link to the Activity list, filtered, for the same period.
const activity = (filters = {}) => route('usage.activity', { days: props.days, ...filters });

// ---- The four numbers, each against the period before ----

// Percent change, or null when there is nothing to compare with.
const change = (now, before) => (before > 0 ? Math.round(((now - before) / before) * 100) : null);

const kpis = computed(() => [
    {
        label: 'Active people',
        value: count(props.summary.people),
        suffix: props.enrolled ? `of ${props.enrolled} enrolled` : null,
        change: change(props.summary.people, props.previous.people),
    },
    { label: 'Prompts', value: count(props.summary.prompts), change: change(props.summary.prompts, props.previous.prompts) },
    {
        label: 'AI time',
        value: duration(props.summary.ai_seconds),
        title: props.aiTimeDefinition,
        change: change(props.summary.ai_seconds, props.previous.ai_seconds),
    },
    { label: 'Active projects', value: count(props.summary.projects), change: change(props.summary.projects, props.previous.projects) },
]);

// ---- Activity chart: prompts per bucket, stacked by tool ----

// Two ids can be one tool to a reader (the Copilot chat extension reports under
// two names), so series merge by display name.
const series = computed(() => {
    const byName = new Map();
    for (const [tool, counts] of Object.entries(props.activity.series)) {
        const name = toolName(tool || null);
        const row = byName.get(name) ?? { name, tool, counts: counts.map(() => 0) };
        counts.forEach((n, i) => (row.counts[i] += n));
        byName.set(name, row);
    }
    const rows = [...byName.values()].map((r) => ({ ...r, total: r.counts.reduce((a, b) => a + b, 0) }));
    return rows.sort((a, b) => b.total - a.total);
});

const columns = computed(() =>
    props.activity.buckets.map((at, i) => {
        const parts = series.value.map((s) => ({ name: s.name, color: toolColor(s.tool), n: s.counts[i] })).filter((p) => p.n > 0);
        return { at, parts, total: parts.reduce((a, p) => a + p.n, 0) };
    }),
);
const maxColumn = computed(() => Math.max(1, ...columns.value.map((c) => c.total)));

// Buckets are cut in UTC on the server. Days, weeks and months are labelled in
// UTC so a bar never claims the neighbouring date; hours show local clock time.
const bucketLabel = (at, long = false) => {
    const d = new Date(at.replace(' ', 'T') + 'Z');
    const unit = props.activity.unit;
    if (unit === 'hour') return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    if (unit === 'month') return d.toLocaleDateString([], { month: 'short', year: long ? 'numeric' : undefined, timeZone: 'UTC' });
    const text = d.toLocaleDateString([], { weekday: unit === 'day' ? 'short' : undefined, day: 'numeric', month: 'short', timeZone: 'UTC' });
    return unit === 'week' && long ? `Week of ${text}` : text;
};
// About six labels along the axis, whatever the bar count.
const labelEvery = computed(() => Math.max(1, Math.ceil(columns.value.length / 6)));
const hovered = ref(null);

// ---- Tools share ----

const toolShare = computed(() => {
    const total = series.value.reduce((a, s) => a + s.total, 0) || 1;
    return series.value.map((s) => ({ ...s, share: Math.round((s.total / total) * 100) }));
});

// ---- Projects, with a trend line each ----

const projects = computed(() => {
    const named = props.perProject.filter((p) => p.repo).sort((a, b) => b.prompts - a.prompts || b.interactions - a.interactions).slice(0, 8);
    // Work outside a checkout is real work, shown last rather than ranked.
    const outside = props.perProject.find((p) => !p.repo);
    return outside ? [...named, outside] : named;
});

const sparkline = (repo) => {
    const counts = props.trends[repo ?? ''] ?? [];
    if (counts.length < 2) return '';
    const max = Math.max(1, ...counts);
    return counts.map((n, i) => `${(i / (counts.length - 1)) * 80},${22 - (n / max) * 20}`).join(' ');
};

// ---- Team ----

const status = (p) => {
    if (!p.last_seen) return { label: 'No activity', dot: 'bg-gray-300 dark:bg-gray-600' };
    const minutes = (Date.now() - new Date(p.last_seen).getTime()) / 60000;
    if (minutes < 15) return { label: 'Active now', dot: 'bg-emerald-500' };
    if (minutes < 24 * 60) return { label: 'Today', dot: 'bg-sky-500' };
    return { label: `Idle ${Math.floor(minutes / 1440)}d`, dot: 'bg-gray-400' };
};

// ---- Needs attention ----

const attentionCount = computed(
    () => props.attention.secrets_total + props.attention.silent.length + props.attention.unassigned.length,
);
const ruleName = (rule) => rule.replace(/[-_]/g, ' ');

const insightText = (i) => (typeof i === 'string' ? i : `${toolName(i.tool)} grew the most: ${count(i.growth)} more prompts than the previous period.`);
</script>

<template>
    <Head title="Overview" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-8">
            <!-- Title and period -->
            <div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
                <div>
                    <h1 class="text-2xl font-semibold tracking-tight text-gray-900">Overview</h1>
                    <p class="mt-1 text-sm text-gray-500">AI use across the team, {{ periodName }}, compared with the period before.</p>
                </div>
                <div class="inline-flex self-start overflow-x-auto rounded-lg border border-gray-200 bg-white p-0.5 shadow-sm" role="group" aria-label="Period">
                    <button
                        v-for="p in periods"
                        :key="p.value"
                        type="button"
                        class="whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition"
                        :class="days === p.value ? 'bg-gray-900 text-white dark:bg-gray-800 dark:text-gray-50' : 'text-gray-600 hover:text-gray-900'"
                        :aria-pressed="days === p.value"
                        @click="setDays(p.value)"
                    >{{ p.label }}</button>
                </div>
            </div>

            <!-- The four numbers -->
            <div class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-5">
                <div v-for="k in kpis" :key="k.label" class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm sm:p-5" :title="k.title">
                    <div class="text-sm text-gray-500">{{ k.label }}</div>
                    <div class="mt-1 flex flex-wrap items-baseline gap-x-2">
                        <span class="text-2xl font-semibold tabular-nums tracking-tight text-gray-900 sm:text-3xl">{{ k.value }}</span>
                        <span v-if="k.suffix" class="text-sm text-gray-500">{{ k.suffix }}</span>
                    </div>
                    <div class="mt-1 text-xs text-gray-500">
                        <template v-if="k.change === null">No earlier data</template>
                        <template v-else-if="k.change === 0">Same as before</template>
                        <template v-else>
                            <span class="font-medium text-gray-700">{{ k.change > 0 ? '▲' : '▼' }} {{ Math.abs(k.change) }}%</span>
                            vs previous
                        </template>
                    </div>
                </div>
            </div>

            <!-- Plain-sentence insights -->
            <ul v-if="insights.length" class="flex flex-col gap-2 rounded-xl border border-indigo-100 bg-indigo-50/60 px-5 py-3.5 text-sm text-gray-700 dark:border-indigo-500/20 dark:bg-indigo-500/10 sm:flex-row sm:flex-wrap sm:gap-x-6">
                <li v-for="(i, n) in insights" :key="n" class="flex gap-2">
                    <span class="text-indigo-500" aria-hidden="true">●</span>{{ insightText(i) }}
                </li>
            </ul>

            <div class="grid gap-6 lg:grid-cols-3">
                <!-- Activity over time -->
                <Panel title="Prompts over time" :subtitle="`Per ${activity.unit}, by tool`" class="lg:col-span-2">
                    <template #actions>
                        <Link :href="activity()" class="text-indigo-600 hover:underline dark:text-indigo-400">View activity →</Link>
                    </template>
                    <div class="px-5 pb-4 pt-3">
                        <!-- Legend, always present: identity is never colour alone -->
                        <ul v-if="series.length" class="mb-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-600">
                            <li v-for="s in series" :key="s.name" class="flex items-center gap-1.5">
                                <span class="h-2.5 w-2.5 rounded-sm" :style="{ background: toolColor(s.tool) }" aria-hidden="true" />{{ s.name }}
                            </li>
                        </ul>

                        <div v-if="!series.length" class="flex h-48 items-center justify-center text-sm text-gray-500">No prompts in this period.</div>
                        <div v-else class="relative">
                            <div class="pointer-events-none absolute inset-x-0 top-0 flex h-48 flex-col justify-between" aria-hidden="true">
                                <div v-for="g in 3" :key="g" class="border-t border-dashed border-gray-100" />
                                <div class="border-t border-gray-200" />
                            </div>
                            <span class="absolute -top-1 right-0 text-[11px] tabular-nums text-gray-400">{{ count(maxColumn) }}</span>

                            <div class="relative flex h-48 items-end gap-[2px]" @mouseleave="hovered = null">
                                <div
                                    v-for="(c, i) in columns"
                                    :key="c.at"
                                    class="group relative flex h-full min-w-0 flex-1 flex-col justify-end"
                                    :aria-label="`${bucketLabel(c.at, true)}: ${c.total} prompts`"
                                    role="img"
                                    @mouseenter="hovered = i"
                                >
                                    <!-- Stack, biggest tool at the bottom, 2px surface gap between parts -->
                                    <div
                                        class="flex flex-col-reverse gap-[2px] overflow-hidden rounded-t transition-opacity"
                                        :class="hovered !== null && hovered !== i ? 'opacity-60' : ''"
                                        :style="{ height: (c.total / maxColumn) * 100 + '%' }"
                                    >
                                        <div v-for="p in c.parts" :key="p.name" :style="{ background: p.color, flexGrow: p.n }" class="min-h-[2px]" />
                                    </div>

                                    <!-- Tooltip -->
                                    <div
                                        v-if="hovered === i"
                                        class="pointer-events-none absolute bottom-full z-10 mb-2 w-44 rounded-lg border border-gray-200 bg-white p-2.5 text-xs shadow-lg"
                                        :class="i > columns.length / 2 ? 'right-0' : 'left-0'"
                                    >
                                        <div class="font-medium text-gray-900">{{ bucketLabel(c.at, true) }}</div>
                                        <div class="mb-1 text-gray-500">{{ count(c.total) }} prompts</div>
                                        <div v-for="p in [...c.parts].reverse()" :key="p.name" class="flex items-center gap-1.5 text-gray-700">
                                            <span class="h-2 w-2 rounded-sm" :style="{ background: p.color }" />
                                            <span class="flex-1 truncate">{{ p.name }}</span>
                                            <span class="tabular-nums">{{ p.n }}</span>
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <div class="mt-1.5 flex gap-[2px] text-[11px] text-gray-400">
                                <div v-for="(c, i) in columns" :key="c.at" class="min-w-0 flex-1 overflow-visible whitespace-nowrap">
                                    <span v-if="i % labelEvery === 0">{{ bucketLabel(c.at) }}</span>
                                </div>
                            </div>
                        </div>
                    </div>
                </Panel>

                <!-- What to act on -->
                <Panel title="Needs attention" :subtitle="attentionCount ? `${attentionCount} item${attentionCount === 1 ? '' : 's'}` : 'All clear'">
                    <div v-if="!attentionCount" class="flex flex-col items-center gap-2 px-5 py-10 text-center">
                        <span class="flex h-9 w-9 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400" aria-hidden="true">✓</span>
                        <p class="text-sm text-gray-600">No secrets caught, every device reporting.</p>
                    </div>

                    <div v-else class="divide-y divide-gray-100">
                        <section v-if="attention.secrets_total" class="px-5 py-3.5">
                            <h4 class="flex items-center gap-2 text-sm font-medium text-gray-900">
                                <span class="flex h-5 w-5 items-center justify-center rounded-full bg-red-100 text-[11px] font-bold text-red-700 dark:bg-red-500/20 dark:text-red-300" aria-hidden="true">!</span>
                                {{ attention.secrets_total }} secret{{ attention.secrets_total === 1 ? '' : 's' }} caught in prompts
                            </h4>
                            <p class="mt-0.5 text-xs text-gray-500">Masked before storage. The key may still need rotating.</p>
                            <ul class="mt-2 space-y-1">
                                <li v-for="s in attention.secrets" :key="s.id">
                                    <Link :href="route('usage.show', s.id)" class="flex items-baseline justify-between gap-2 rounded-md px-1.5 py-1 text-sm hover:bg-gray-50">
                                        <span class="min-w-0 truncate text-gray-700">
                                            {{ s.person }} <span class="text-gray-400">· {{ (s.rules ?? []).map(ruleName).join(', ') }}</span>
                                        </span>
                                        <span class="shrink-0 text-xs text-gray-400" :title="when(s.occurred_at)">{{ ago(s.occurred_at) }}</span>
                                    </Link>
                                </li>
                            </ul>
                        </section>

                        <section v-if="attention.silent.length" class="px-5 py-3.5">
                            <h4 class="flex items-center gap-2 text-sm font-medium text-gray-900">
                                <span class="flex h-5 w-5 items-center justify-center rounded-full bg-amber-100 text-[11px] font-bold text-amber-700 dark:bg-amber-500/20 dark:text-amber-300" aria-hidden="true">!</span>
                                {{ attention.silent.length }} device{{ attention.silent.length === 1 ? '' : 's' }} silent for 3+ days
                            </h4>
                            <p class="mt-0.5 text-xs text-gray-500">Nothing captured: the agent may be off, or no AI was used.</p>
                            <ul class="mt-2 space-y-1 text-sm">
                                <li v-for="d in attention.silent.slice(0, 5)" :key="d.id" class="flex items-baseline justify-between gap-2 px-1.5">
                                    <span class="min-w-0 truncate text-gray-700">{{ d.person ?? 'No one' }} <span class="text-gray-400">· {{ d.hostname }}</span></span>
                                    <span class="shrink-0 text-xs text-gray-400">{{ d.last_seen_at ? ago(d.last_seen_at) : 'never' }}</span>
                                </li>
                            </ul>
                        </section>

                        <section v-if="attention.unassigned.length" class="px-5 py-3.5">
                            <h4 class="flex items-center gap-2 text-sm font-medium text-gray-900">
                                <span class="flex h-5 w-5 items-center justify-center rounded-full bg-amber-100 text-[11px] font-bold text-amber-700 dark:bg-amber-500/20 dark:text-amber-300" aria-hidden="true">!</span>
                                {{ attention.unassigned.length }} device{{ attention.unassigned.length === 1 ? '' : 's' }} not linked to a person
                            </h4>
                            <p class="mt-0.5 text-xs text-gray-500">Its work shows as "Unassigned device". Link it on the People page.</p>
                            <ul class="mt-2 space-y-1 text-sm">
                                <li v-for="d in attention.unassigned.slice(0, 5)" :key="d.id" class="px-1.5 text-gray-700">{{ d.hostname }}</li>
                            </ul>
                        </section>
                    </div>
                </Panel>
            </div>

            <div class="grid gap-6 lg:grid-cols-3">
                <!-- Projects -->
                <Panel title="Projects" subtitle="The repository the work happened in" class="lg:col-span-2">
                    <div v-if="!projects.length" class="px-5 py-10 text-center text-sm text-gray-500">No project work in this period.</div>
                    <table v-else class="w-full text-sm">
                        <thead>
                            <tr class="border-b border-gray-100 text-left text-xs font-medium text-gray-500">
                                <th class="px-5 py-2.5 font-medium">Project</th>
                                <th class="hidden px-3 py-2.5 text-right font-medium sm:table-cell">People</th>
                                <th class="px-3 py-2.5 text-right font-medium">Prompts</th>
                                <th class="hidden px-3 py-2.5 text-right font-medium md:table-cell">AI time</th>
                                <th class="hidden px-3 py-2.5 font-medium sm:table-cell">Trend</th>
                                <th class="px-5 py-2.5 text-right font-medium">Last active</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr
                                v-for="p in projects"
                                :key="p.repo ?? '-'"
                                class="cursor-pointer transition hover:bg-gray-50"
                                @click="router.visit(activity({ project: p.repo ?? '-' }))"
                            >
                                <td class="max-w-0 px-5 py-2.5">
                                    <Link
                                        :href="activity({ project: p.repo ?? '-' })"
                                        class="block truncate"
                                        :class="p.repo ? 'font-medium text-gray-900' : 'italic text-gray-500'"
                                        :title="p.repo ?? 'Browser chats and tools run outside a repository'"
                                        @click.stop
                                    >{{ p.name ?? 'Outside a project' }}</Link>
                                </td>
                                <td class="hidden px-3 py-2.5 text-right tabular-nums text-gray-600 sm:table-cell">{{ p.people }}</td>
                                <td class="px-3 py-2.5 text-right tabular-nums text-gray-900">{{ count(p.prompts) }}</td>
                                <td class="hidden px-3 py-2.5 text-right tabular-nums text-gray-600 md:table-cell">{{ duration(p.ai_seconds) }}</td>
                                <td class="hidden px-3 py-2.5 sm:table-cell">
                                    <svg v-if="sparkline(p.repo)" viewBox="0 0 80 24" class="h-6 w-20" aria-hidden="true">
                                        <polyline :points="sparkline(p.repo)" fill="none" stroke="var(--series-1)" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
                                    </svg>
                                </td>
                                <td class="whitespace-nowrap px-5 py-2.5 text-right text-xs text-gray-500" :title="when(p.last_seen)">{{ ago(p.last_seen) }}</td>
                            </tr>
                        </tbody>
                    </table>
                </Panel>

                <!-- Tools share -->
                <Panel title="Tools" subtitle="Share of prompts">
                    <ul v-if="toolShare.length" class="space-y-3 px-5 py-4">
                        <li v-for="t in toolShare" :key="t.name">
                            <Link :href="activity({ tool: t.tool })" class="block rounded-md hover:bg-gray-50">
                                <div class="flex justify-between text-sm">
                                    <span class="text-gray-700">{{ t.name }}</span>
                                    <span class="tabular-nums text-gray-500">{{ t.share }}% · {{ count(t.total) }}</span>
                                </div>
                                <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-gray-100">
                                    <div class="h-full rounded-full" :style="{ width: t.share + '%', background: toolColor(t.tool) }" />
                                </div>
                            </Link>
                        </li>
                    </ul>
                    <p v-else class="px-5 py-10 text-center text-sm text-gray-500">No prompts yet.</p>
                </Panel>
            </div>

            <!-- Team -->
            <Panel title="Team" :subtitle="`${summary.people} of ${enrolled || team.length} active in this period`">
                <div v-if="!team.length" class="px-5 py-10 text-center text-sm text-gray-500">No one yet. Add people and pair their devices.</div>
                <table v-else class="w-full text-sm">
                    <thead>
                        <tr class="border-b border-gray-100 text-left text-xs text-gray-500">
                            <th class="px-5 py-2.5 font-medium">Person</th>
                            <th class="px-3 py-2.5 font-medium">Status</th>
                            <th class="px-3 py-2.5 text-right font-medium">Prompts</th>
                            <th class="hidden px-3 py-2.5 text-right font-medium sm:table-cell">AI time</th>
                            <th class="hidden px-3 py-2.5 font-medium md:table-cell">Main tool</th>
                            <th class="hidden px-5 py-2.5 text-right font-medium sm:table-cell">Last seen</th>
                        </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-100">
                        <tr
                            v-for="p in team"
                            :key="p.user_id ?? 'none'"
                            class="transition"
                            :class="p.user_id ? 'cursor-pointer hover:bg-gray-50' : ''"
                            @click="p.user_id && router.visit(activity({ person: p.user_id }))"
                        >
                            <td class="max-w-0 px-5 py-2.5">
                                <div class="flex items-center gap-2.5">
                                    <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-[11px] font-semibold text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300" aria-hidden="true">{{ initials(p.name) }}</span>
                                    <Link v-if="p.user_id" :href="activity({ person: p.user_id })" class="truncate font-medium text-gray-900" @click.stop>{{ p.name }}</Link>
                                    <span v-else class="truncate italic text-gray-500">{{ p.name }}</span>
                                </div>
                            </td>
                            <td class="whitespace-nowrap px-3 py-2.5">
                                <span class="inline-flex items-center gap-1.5 text-gray-700">
                                    <span class="h-2 w-2 rounded-full" :class="status(p).dot" aria-hidden="true" />{{ status(p).label }}
                                </span>
                            </td>
                            <td class="px-3 py-2.5 text-right tabular-nums text-gray-900">{{ count(p.prompts) }}</td>
                            <td class="hidden px-3 py-2.5 text-right tabular-nums text-gray-600 sm:table-cell">{{ duration(p.ai_seconds) }}</td>
                            <td class="hidden px-3 py-2.5 md:table-cell">
                                <span v-if="p.main_tool" class="inline-flex items-center gap-1.5 text-gray-700">
                                    <span class="h-2 w-2 rounded-sm" :style="{ background: toolColor(p.main_tool) }" aria-hidden="true" />{{ toolName(p.main_tool) }}
                                </span>
                                <span v-else class="text-gray-400">—</span>
                            </td>
                            <td class="hidden whitespace-nowrap px-5 py-2.5 text-right text-xs text-gray-500 sm:table-cell" :title="when(p.last_seen)">{{ ago(p.last_seen) }}</td>
                        </tr>
                    </tbody>
                </table>
            </Panel>

            <!-- Latest prompts -->
            <Panel title="Latest prompts">
                <template #actions>
                    <Link :href="activity()" class="text-indigo-600 hover:underline dark:text-indigo-400">View all activity →</Link>
                </template>
                <ul class="divide-y divide-gray-100">
                    <li v-for="p in latest" :key="p.id">
                        <Link :href="route('usage.session', p.session_id) + '#i-' + p.id" class="flex gap-3 px-5 py-3 transition hover:bg-gray-50">
                            <div class="min-w-0 flex-1">
                                <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                                    <span class="font-medium text-gray-900">{{ p.person ?? 'Unassigned device' }}</span>
                                    <Tag :label="toolName(p.tool)" tone="blue" />
                                    <span v-if="p.project" class="truncate text-gray-500">{{ p.project }}</span>
                                    <span class="ml-auto shrink-0 text-xs text-gray-400" :title="when(p.occurred_at)">{{ ago(p.occurred_at) }}</span>
                                </div>
                                <p v-if="p.preview" class="mt-1 line-clamp-1 text-sm text-gray-600">{{ p.preview }}</p>
                            </div>
                        </Link>
                    </li>
                    <li v-if="!latest.length" class="px-5 py-8 text-center text-sm text-gray-500">No prompts in this period.</li>
                </ul>
            </Panel>
        </div>
    </AuthenticatedLayout>
</template>
