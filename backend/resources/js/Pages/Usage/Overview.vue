<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { ago, count, duration, initials, toolName, when } from '@/Components/Usage/format';
import { Head, Link, router } from '@inertiajs/vue3';
import { computed, ref } from 'vue';

const props = defineProps({
    days: Number,
    summary: Object,
    previous: Object,
    enrolled: Number,
    leaks: Object,
    perProject: Array,
    team: Array,
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
const activityLink = (filters = {}) => route('usage.activity', { days: props.days, ...filters });

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

// ---- Team: the heart of the page ----

const status = (p) => {
    if (!p.last_seen) return { label: 'No activity', dot: 'bg-gray-300 dark:bg-gray-600', rank: 3 };
    const minutes = (Date.now() - new Date(p.last_seen).getTime()) / 60000;
    if (minutes < 15) return { label: 'Active now', dot: 'bg-emerald-500', rank: 0 };
    if (minutes < 24 * 60) return { label: 'Today', dot: 'bg-sky-500', rank: 1 };
    return { label: `Idle ${Math.floor(minutes / 1440)}d`, dot: 'bg-gray-400', rank: 2 };
};
const stepsPerPrompt = (p) => (p.prompts > 0 ? Math.round((p.agent_steps / p.prompts) * 10) / 10 : null);

const columns = [
    { key: 'name', label: 'Person', value: (p) => p.name.toLowerCase(), align: 'left' },
    { key: 'status', label: 'Status', value: (p) => -status(p).rank, align: 'left' },
    { key: 'prompts', label: 'Prompts', value: (p) => p.prompts },
    { key: 'change', label: 'Change', value: (p) => change(p.prompts, p.prompts_before) ?? -Infinity, hide: 'hidden sm:table-cell', title: 'Prompts compared with the period before' },
    { key: 'ai_seconds', label: 'AI time', value: (p) => p.ai_seconds, hide: 'hidden md:table-cell' },
    { key: 'main_project', label: 'Main project', value: (p) => (p.main_project ?? '').toLowerCase(), align: 'left', hide: 'hidden lg:table-cell' },
    { key: 'active_days', label: 'Active days', value: (p) => p.active_days, hide: 'hidden lg:table-cell', title: 'Days with any AI use in this period' },
    { key: 'steps', label: 'AI steps / prompt', value: (p) => stepsPerPrompt(p) ?? -1, hide: 'hidden xl:table-cell', title: 'Actions the AI took on its own for each request: higher means more work handed over' },
    { key: 'last_seen', label: 'Last seen', value: (p) => (p.last_seen ? new Date(p.last_seen).getTime() : 0), hide: 'hidden sm:table-cell' },
];

const search = ref('');
const sortKey = ref('prompts');
const sortDesc = ref(true);
const sortBy = (key) => {
    sortDesc.value = sortKey.value === key ? !sortDesc.value : key !== 'name';
    sortKey.value = key;
};
const people = computed(() => {
    const q = search.value.trim().toLowerCase();
    const col = columns.find((c) => c.key === sortKey.value);
    return props.team
        .filter((p) => !q || p.name.toLowerCase().includes(q) || (p.main_project ?? '').toLowerCase().includes(q))
        .sort((a, b) => {
            const [x, y] = [col.value(a), col.value(b)];
            return (x < y ? -1 : x > y ? 1 : 0) * (sortDesc.value ? -1 : 1);
        });
});

// ---- Projects: what the AI work went into, and who did it ----

const projects = computed(() => {
    const named = props.perProject.filter((p) => p.repo).sort((a, b) => b.prompts - a.prompts || b.interactions - a.interactions);
    // Work outside a checkout is real work, shown last rather than ranked.
    const outside = props.perProject.find((p) => !p.repo);
    return outside ? [...named, outside] : named;
});
const share = (p) => (props.summary.prompts > 0 ? Math.round((p.prompts / props.summary.prompts) * 100) : 0);
</script>

<template>
    <Head title="AI adoption" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-8">
            <!-- Title and period -->
            <div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
                <div>
                    <h1 class="text-2xl font-semibold tracking-tight text-gray-900">AI adoption</h1>
                    <p class="mt-1 text-sm text-gray-500">Who uses AI, on which projects, {{ periodName }}, against the period before.</p>
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

            <!-- The one alarm: only when a working credential reached an AI tool -->
            <section v-if="leaks.total" class="rounded-xl border border-red-200 bg-red-50 px-5 py-4 dark:border-red-500/30 dark:bg-red-500/10" role="alert">
                <h2 class="flex items-center gap-2 text-sm font-semibold text-red-800 dark:text-red-300">
                    <span class="flex h-5 w-5 items-center justify-center rounded-full bg-red-600 text-[11px] font-bold text-white" aria-hidden="true">!</span>
                    {{ leaks.total }} credential{{ leaks.total === 1 ? '' : 's' }} pasted into AI tools — rotate {{ leaks.total === 1 ? 'it' : 'them' }}
                </h2>
                <p class="mt-1 text-xs text-red-700/80 dark:text-red-300/80">Masked before storage, but the AI provider received the prompt as it was sent.</p>
                <ul class="mt-2 flex flex-wrap gap-2">
                    <li v-for="l in leaks.items" :key="l.id">
                        <Link :href="route('usage.show', l.id)" class="inline-flex items-center gap-1.5 rounded-md bg-white px-2.5 py-1 text-xs text-gray-700 ring-1 ring-red-200 hover:ring-red-400 dark:ring-red-500/30">
                            <span class="font-medium text-gray-900">{{ l.person }}</span>
                            {{ l.rules.join(', ').replace(/-/g, ' ') }} · {{ ago(l.occurred_at) }}
                        </Link>
                    </li>
                </ul>
            </section>

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
            <ul v-if="insights.length" class="flex flex-col gap-2 rounded-xl border border-indigo-100 bg-indigo-50/60 px-5 py-3.5 text-sm text-gray-700 dark:border-indigo-500/20 dark:bg-indigo-500/10 lg:flex-row lg:flex-wrap lg:gap-x-6">
                <li v-for="(i, n) in insights" :key="n" class="flex gap-2">
                    <span class="text-indigo-500" aria-hidden="true">●</span>{{ i }}
                </li>
            </ul>

            <!-- Team -->
            <Panel title="Team" :subtitle="`${summary.people} of ${enrolled || team.length} people used AI in this period`">
                <template #actions>
                    <div class="flex items-center gap-3">
                        <input
                            v-model="search"
                            type="search"
                            placeholder="Find a person or project"
                            aria-label="Find a person or project"
                            class="hidden w-56 rounded-lg px-3 py-1.5 text-sm sm:block"
                        />
                        <Link :href="activityLink()" class="whitespace-nowrap text-indigo-600 hover:underline dark:text-indigo-400">All activity →</Link>
                    </div>
                </template>
                <input
                    v-model="search"
                    type="search"
                    placeholder="Find a person or project"
                    aria-label="Find a person or project"
                    class="mx-5 mt-3 block w-[calc(100%-2.5rem)] rounded-lg px-3 py-1.5 text-sm sm:hidden"
                />

                <div v-if="!team.length" class="px-5 py-10 text-center text-sm text-gray-500">No one yet. Add people and pair their devices.</div>
                <table v-else class="w-full text-sm">
                    <thead>
                        <tr class="border-b border-gray-100 text-xs text-gray-500">
                            <th
                                v-for="c in columns"
                                :key="c.key"
                                class="px-3 py-2.5 font-medium first:pl-5 last:pr-5"
                                :class="[c.hide, c.align === 'left' ? 'text-left' : 'text-right']"
                                :title="c.title"
                                :aria-sort="sortKey === c.key ? (sortDesc ? 'descending' : 'ascending') : 'none'"
                            >
                                <button type="button" class="inline-flex items-center gap-1 hover:text-gray-900" @click="sortBy(c.key)">
                                    {{ c.label }}
                                    <span class="w-2 text-[10px]" aria-hidden="true">{{ sortKey === c.key ? (sortDesc ? '▼' : '▲') : '' }}</span>
                                </button>
                            </th>
                        </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-100">
                        <tr
                            v-for="p in people"
                            :key="p.user_id ?? 'none'"
                            class="transition"
                            :class="p.user_id ? 'cursor-pointer hover:bg-gray-50' : ''"
                            @click="p.user_id && router.visit(activityLink({ person: p.user_id }))"
                        >
                            <td class="max-w-[10rem] py-2.5 pl-5 pr-3 sm:max-w-[16rem]">
                                <div class="flex items-center gap-2.5">
                                    <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-[11px] font-semibold text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300" aria-hidden="true">{{ initials(p.name) }}</span>
                                    <Link v-if="p.user_id" :href="activityLink({ person: p.user_id })" class="truncate font-medium text-gray-900" @click.stop>{{ p.name }}</Link>
                                    <span v-else class="truncate italic text-gray-500">{{ p.name }}</span>
                                </div>
                            </td>
                            <td class="whitespace-nowrap px-3 py-2.5">
                                <span class="inline-flex items-center gap-1.5 text-gray-700">
                                    <span class="h-2 w-2 rounded-full" :class="status(p).dot" aria-hidden="true" />{{ status(p).label }}
                                </span>
                            </td>
                            <td class="px-3 py-2.5 text-right font-medium tabular-nums text-gray-900">{{ count(p.prompts) }}</td>
                            <td class="hidden whitespace-nowrap px-3 py-2.5 text-right tabular-nums sm:table-cell">
                                <span v-if="change(p.prompts, p.prompts_before) === null" class="text-xs text-gray-400">{{ p.prompts ? 'new' : '—' }}</span>
                                <span v-else-if="change(p.prompts, p.prompts_before) === 0" class="text-xs text-gray-400">same</span>
                                <span v-else class="text-xs font-medium" :class="change(p.prompts, p.prompts_before) > 0 ? 'text-gray-700' : 'text-amber-700 dark:text-amber-400'">
                                    {{ change(p.prompts, p.prompts_before) > 0 ? '▲' : '▼' }} {{ Math.abs(change(p.prompts, p.prompts_before)) }}%
                                </span>
                            </td>
                            <td class="hidden whitespace-nowrap px-3 py-2.5 text-right tabular-nums text-gray-600 md:table-cell">{{ duration(p.ai_seconds) }}</td>
                            <td class="hidden max-w-[12rem] px-3 py-2.5 lg:table-cell">
                                <span class="block truncate" :class="p.main_project ? 'text-gray-700' : 'text-gray-400'" :title="p.main_tool ? `Mostly ${toolName(p.main_tool)}` : null">{{ p.main_project ?? (p.prompts ? 'General chat' : '—') }}</span>
                            </td>
                            <td class="hidden px-3 py-2.5 text-right tabular-nums text-gray-600 lg:table-cell">{{ p.active_days || '—' }}</td>
                            <td class="hidden px-3 py-2.5 text-right tabular-nums text-gray-600 xl:table-cell">{{ stepsPerPrompt(p) ?? '—' }}</td>
                            <td class="hidden whitespace-nowrap py-2.5 pl-3 pr-5 text-right text-xs text-gray-500 sm:table-cell" :title="when(p.last_seen)">{{ p.last_seen ? ago(p.last_seen) : '—' }}</td>
                        </tr>
                        <tr v-if="!people.length">
                            <td :colspan="columns.length" class="px-5 py-8 text-center text-sm text-gray-500">No one matches “{{ search }}”.</td>
                        </tr>
                    </tbody>
                </table>
            </Panel>

            <!-- Projects -->
            <Panel title="Projects" subtitle="What the AI work went into, and who did it">
                <div v-if="!projects.length" class="px-5 py-10 text-center text-sm text-gray-500">No AI work in this period.</div>
                <ul v-else class="divide-y divide-gray-100">
                    <li v-for="p in projects" :key="p.repo ?? '-'">
                        <Link
                            :href="activityLink({ project: p.repo ?? '-' })"
                            class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 px-5 py-3 transition hover:bg-gray-50 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_9rem_5rem]"
                            :title="p.repo ?? 'Browser chats and tools run outside a repository'"
                        >
                            <span class="truncate text-sm" :class="p.repo ? 'font-medium text-gray-900' : 'italic text-gray-500'">{{ p.name ?? 'General chat (no project)' }}</span>

                            <!-- Share of the team's prompts -->
                            <div class="order-last col-span-2 flex items-center gap-2 sm:order-none sm:col-span-1">
                                <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-gray-100">
                                    <div class="h-full rounded-full bg-indigo-500" :style="{ width: share(p) + '%' }" />
                                </div>
                                <span class="w-9 text-right text-xs tabular-nums text-gray-500">{{ share(p) }}%</span>
                            </div>

                            <!-- Who worked on it -->
                            <div class="flex items-center justify-end gap-1" :title="p.contributors.map((c) => `${c.name} (${c.prompts})`).join(', ')">
                                <span
                                    v-for="c in p.contributors.slice(0, 4)"
                                    :key="c.user_id ?? 'none'"
                                    class="flex h-6 w-6 items-center justify-center rounded-full bg-indigo-100 text-[10px] font-semibold text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300"
                                >{{ initials(c.name) }}</span>
                                <span v-if="p.contributors.length > 4" class="flex h-6 items-center rounded-full bg-gray-100 px-1.5 text-[10px] font-medium text-gray-600">+{{ p.contributors.length - 4 }}</span>
                            </div>

                            <div class="hidden text-right sm:block">
                                <div class="text-sm tabular-nums text-gray-900">{{ count(p.prompts) }}</div>
                                <div class="text-[11px] text-gray-500">
                                    <template v-if="change(p.prompts, p.prompts_before) === null">new</template>
                                    <template v-else>{{ change(p.prompts, p.prompts_before) >= 0 ? '▲' : '▼' }} {{ Math.abs(change(p.prompts, p.prompts_before)) }}%</template>
                                </div>
                            </div>
                        </Link>
                    </li>
                </ul>
            </Panel>

            <p class="px-1 text-xs leading-relaxed text-gray-500">
                These numbers show how much AI is used, not how well anyone works: more prompts is not better work.
                <strong class="font-medium text-gray-700">AI time</strong> — {{ aiTimeDefinition }}
            </p>
        </div>
    </AuthenticatedLayout>
</template>
