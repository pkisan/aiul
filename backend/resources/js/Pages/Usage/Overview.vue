<script setup>
// The manager's page. It answers three questions, in this order:
//   1. Is my team using AI?        one sentence and one bar
//   2. Who needs me?               a short list, each item with what to do
//   3. Who does what, and where?   the people table and the projects list
// Volume (prompts, time) is context, not the headline: more prompts is not
// better work.
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
    attention: Array,
    habitDays: Object,
    aiTimeDefinition: String,
});

const periods = [
    { value: 1, label: 'Today' },
    { value: 7, label: '7 days' },
    { value: 30, label: '30 days' },
    { value: 90, label: '90 days' },
    { value: 365, label: '1 year' },
];
const periodName = computed(() => (props.days === 1 ? 'today' : `in the last ${props.days} days`));
const setDays = (days) => router.get(route('usage.index'), days === 7 ? {} : { days }, { preserveScroll: true });
const activityLink = (filters = {}) => route('usage.activity', { days: props.days, ...filters });
const change = (now, before) => (before > 0 ? Math.round(((now - before) / before) * 100) : null);

// ---- 1. Is my team using AI? ----

// One ordinal scale, darkest = most often. Light and dark mode have their own
// steps (checked with the dataviz palette validator against each surface).
const habitOrder = ['most', 'some', 'rare', 'none'];
const habits = computed(() => {
    const d = props.days;
    const most = props.habitDays.most;
    const some = props.habitDays.some;
    const range = (from, to) => (from === to ? `${from}` : `${from}–${to}`);
    return {
        most: {
            label: d === 1 ? 'Used today' : 'Most days',
            meaning: d === 1 ? '' : `${range(most, d)} of ${d} days`,
            bar: 'bg-indigo-800 dark:bg-indigo-400',
        },
        some: { label: 'Some days', meaning: `${range(some, most - 1)} days`, bar: 'bg-indigo-600 dark:bg-indigo-500' },
        rare: { label: 'Rarely', meaning: `${range(1, some - 1)} day${some - 1 === 1 ? '' : 's'}`, bar: 'bg-indigo-400 dark:bg-indigo-700' },
        none: { label: 'Not used', meaning: '', bar: 'bg-gray-200' },
    };
});

// People only: an unassigned device is not a person to count.
const people = computed(() => props.team.filter((p) => p.user_id));
const groups = computed(() =>
    habitOrder
        // Over one day there is no "some days" or "rarely".
        .filter((key) => props.days > 1 || key === 'most' || key === 'none')
        .map((key) => ({ key, ...habits.value[key], people: people.value.filter((p) => p.habit === key) })),
);
const usedCount = computed(() => people.value.filter((p) => p.habit !== 'none').length);
const mostCount = computed(() => people.value.filter((p) => p.habit === 'most').length);
const mostBefore = computed(() => people.value.filter((p) => p.habit_before === 'most').length);

const headline = computed(() => {
    if (!people.value.length) return 'No one is set up yet.';
    return `${usedCount.value} of ${people.value.length} people used AI ${periodName.value}.`;
});
const subline = computed(() => {
    if (props.days === 1 || !usedCount.value) return null;
    const n = mostCount.value;
    const who = n === 1 ? '1 person uses' : `${n} people use`;
    // No data before this period (a new install): nothing to compare with.
    if (!props.previous.prompts) return `${who} it most days.`;
    const was = mostBefore.value === n ? 'same as the period before' : `${mostBefore.value} the period before`;
    return `${who} it most days (${was}).`;
});

// Volume, as context beside the headline.
const volume = computed(() => [
    { label: 'Prompts', value: count(props.summary.prompts), change: change(props.summary.prompts, props.previous.prompts), title: 'Requests people typed into an AI tool' },
    { label: 'Time with AI', value: duration(props.summary.ai_seconds), change: change(props.summary.ai_seconds, props.previous.ai_seconds), title: props.aiTimeDefinition },
    { label: 'Projects', value: count(props.summary.projects), change: change(props.summary.projects, props.previous.projects), title: 'Code repositories AI was used in' },
]);

// ---- 2. Who needs me? ----

const attentionIcon = {
    dropped: { glyph: '↓', ring: 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300' },
    not_started: { glyph: '○', ring: 'bg-gray-100 text-gray-600' },
    one_person: { glyph: '1', ring: 'bg-sky-100 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300' },
};
const attentionLink = (a) => {
    if (a.kind === 'one_person') return activityLink({ project: a.repo });
    if (a.kind === 'dropped') return activityLink({ person: a.user_id, days: Math.max(props.days * 2, 7) });
    return null;
};

// ---- 3. Who does what ----

// The days of the period as UTC dates, oldest first. UTC because the server
// buckets days in UTC too (a known ceiling, see PROGRESS).
const periodDates = computed(() =>
    Array.from({ length: props.days }, (_, i) => new Date(Date.now() - (props.days - 1 - i) * 86400000).toISOString().slice(0, 10)),
);
// One cell per day reads at a glance up to two weeks; past that, a number.
const showStrip = computed(() => props.days > 1 && props.days <= 14);

const columns = [
    { key: 'name', label: 'Person', value: (p) => p.name.toLowerCase(), align: 'left' },
    { key: 'habit', label: 'How often', value: (p) => p.active_days * 1000 + p.prompts, align: 'left', title: 'Days with any AI use in this period' },
    { key: 'prompts', label: 'Prompts', value: (p) => p.prompts, hide: 'hidden sm:table-cell' },
    { key: 'main_project', label: 'Mostly on', value: (p) => (p.main_project ?? '').toLowerCase(), align: 'left', hide: 'hidden md:table-cell' },
    { key: 'last_seen', label: 'Last used', value: (p) => (p.last_seen ? new Date(p.last_seen).getTime() : 0), hide: 'hidden lg:table-cell' },
];

const search = ref('');
const sortKey = ref('habit');
const sortDesc = ref(true);
const sortBy = (key) => {
    sortDesc.value = sortKey.value === key ? !sortDesc.value : key !== 'name' && key !== 'main_project';
    sortKey.value = key;
};
const rows = computed(() => {
    const q = search.value.trim().toLowerCase();
    const col = columns.find((c) => c.key === sortKey.value);
    return props.team
        .filter((p) => !q || p.name.toLowerCase().includes(q) || (p.main_project ?? '').toLowerCase().includes(q))
        .sort((a, b) => {
            // An unassigned device is not a person: always last.
            if (!a.user_id !== !b.user_id) return a.user_id ? -1 : 1;
            const [x, y] = [col.value(a), col.value(b)];
            return (x < y ? -1 : x > y ? 1 : 0) * (sortDesc.value ? -1 : 1);
        });
});

// ---- Projects ----

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
        <div class="mx-auto max-w-6xl space-y-6 px-4 py-6 sm:px-6 lg:px-8">
            <!-- Title and period -->
            <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <h1 class="text-2xl font-semibold tracking-tight text-gray-900">AI adoption</h1>
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

            <!-- 1. Is my team using AI? -->
            <section class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm sm:p-6" aria-labelledby="adoption-headline">
                <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_13rem] lg:gap-10">
                    <div>
                        <h2 id="adoption-headline" class="text-xl font-semibold tracking-tight text-gray-900 sm:text-2xl">{{ headline }}</h2>
                        <p v-if="subline" class="mt-1 text-sm text-gray-600 sm:text-base">{{ subline }}</p>

                        <!-- One bar: everyone, split by how often they used AI -->
                        <div v-if="people.length" class="mt-5 flex h-3 gap-0.5 overflow-hidden rounded-full" role="img" :aria-label="groups.map((g) => `${g.label}: ${g.people.length}`).join(', ')">
                            <div
                                v-for="g in groups.filter((g) => g.people.length)"
                                :key="g.key"
                                :class="g.bar"
                                :style="{ flexGrow: g.people.length }"
                                :title="`${g.label}: ${g.people.length}`"
                            />
                        </div>

                        <!-- Who is in each group: the bar's legend and its detail in one -->
                        <div v-if="people.length" class="mt-4 grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-4">
                            <div v-for="g in groups" :key="g.key">
                                <div class="flex items-center gap-2">
                                    <span class="h-2.5 w-2.5 shrink-0 rounded-sm" :class="g.bar" aria-hidden="true" />
                                    <span class="text-sm font-medium text-gray-900">{{ g.label }}</span>
                                    <span class="text-sm tabular-nums text-gray-500">{{ g.people.length }}</span>
                                </div>
                                <p v-if="g.meaning" class="mt-0.5 pl-[18px] text-xs text-gray-500">{{ g.meaning }}</p>
                                <p class="mt-2 pl-[18px] text-sm leading-relaxed text-gray-700">
                                    <template v-for="(p, i) in g.people.slice(0, 6)" :key="p.user_id">
                                        <Link :href="activityLink({ person: p.user_id })" class="hover:text-indigo-600 hover:underline dark:hover:text-indigo-400">{{ p.name }}</Link><span v-if="i < Math.min(g.people.length, 6) - 1">, </span>
                                    </template>
                                    <span v-if="g.people.length > 6" class="text-gray-500"> and {{ g.people.length - 6 }} more</span>
                                    <span v-if="!g.people.length" class="text-xs text-gray-400">No one</span>
                                </p>
                            </div>
                        </div>
                    </div>

                    <!-- Volume, as context -->
                    <dl class="grid grid-cols-3 gap-4 border-t border-gray-100 pt-5 lg:grid-cols-1 lg:content-start lg:gap-5 lg:border-l lg:border-t-0 lg:pl-8 lg:pt-0">
                        <div v-for="v in volume" :key="v.label" :title="v.title">
                            <dt class="text-xs text-gray-500">{{ v.label }}</dt>
                            <dd class="mt-0.5 flex flex-wrap items-baseline gap-x-2">
                                <span class="text-lg font-semibold tabular-nums text-gray-900">{{ v.value }}</span>
                                <span v-if="v.change" class="text-xs tabular-nums text-gray-500">{{ v.change > 0 ? '▲' : '▼' }} {{ Math.abs(v.change) }}%</span>
                            </dd>
                        </div>
                    </dl>
                </div>
            </section>

            <!-- 2. Who needs me? -->
            <section aria-labelledby="attention-title">
                <h2 id="attention-title" class="mb-3 text-sm font-semibold text-gray-900">
                    Needs your attention
                    <span v-if="attention.length" class="ml-1 rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium tabular-nums text-gray-600">{{ attention.length }}</span>
                </h2>
                <p v-if="!attention.length" class="rounded-xl border border-dashed border-gray-200 px-5 py-6 text-center text-sm text-gray-500">
                    <template v-if="days < 7">One day is too short to judge a habit. Pick 7 days or more.</template>
                    <template v-else>Nothing needs you {{ periodName }}.</template>
                </p>
                <ul v-else class="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
                    <li v-for="(a, n) in attention" :key="n" class="flex gap-3 rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
                        <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-semibold" :class="attentionIcon[a.kind].ring" aria-hidden="true">{{ attentionIcon[a.kind].glyph }}</span>
                        <div class="min-w-0">
                            <h3 class="text-sm font-semibold text-gray-900">{{ a.title }}</h3>
                            <p class="mt-0.5 text-sm text-gray-600">{{ a.detail }}</p>
                            <p v-if="a.names.length > 1" class="mt-1 text-sm text-gray-700">{{ a.names.join(', ') }}</p>
                            <p class="mt-2 text-xs leading-relaxed text-gray-500">{{ a.action }}</p>
                            <Link v-if="attentionLink(a)" :href="attentionLink(a)" class="mt-2 inline-block text-xs font-medium text-indigo-600 hover:underline dark:text-indigo-400">
                                {{ a.kind === 'one_person' ? 'See the project' : 'See their activity' }} →
                            </Link>
                        </div>
                    </li>
                </ul>
            </section>

            <!-- 3. Who does what -->
            <Panel title="People" :subtitle="`Everyone with a device, ${periodName}`">
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
                            v-for="p in rows"
                            :key="p.user_id ?? 'none'"
                            class="transition"
                            :class="p.user_id ? 'cursor-pointer hover:bg-gray-50' : ''"
                            @click="p.user_id && router.visit(activityLink({ person: p.user_id }))"
                        >
                            <td class="max-w-[11rem] py-3 pl-5 pr-3 sm:max-w-[16rem]">
                                <div class="flex items-center gap-2.5">
                                    <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-[11px] font-semibold text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300" aria-hidden="true">{{ initials(p.name) }}</span>
                                    <Link v-if="p.user_id" :href="activityLink({ person: p.user_id })" class="truncate font-medium text-gray-900" @click.stop>{{ p.name }}</Link>
                                    <span v-else class="truncate italic text-gray-500">{{ p.name }}</span>
                                </div>
                            </td>
                            <td class="px-3 py-3">
                                <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                                    <span v-if="p.user_id" class="inline-flex w-24 items-center gap-1.5 whitespace-nowrap text-gray-700">
                                        <span class="h-2 w-2 shrink-0 rounded-sm" :class="habits[p.habit].bar" aria-hidden="true" />{{ habits[p.habit].label }}
                                    </span>
                                    <span v-else class="w-24 text-xs text-gray-400" title="Not linked to a person yet">—</span>
                                    <span v-if="showStrip" class="hidden gap-0.5 sm:inline-flex" :title="`${p.active_days} of ${days} days, oldest on the left`">
                                        <span
                                            v-for="d in periodDates"
                                            :key="d"
                                            class="h-3.5 w-2 rounded-sm"
                                            :class="p.days.includes(d) ? 'bg-indigo-600 dark:bg-indigo-400' : 'bg-gray-100'"
                                        />
                                    </span>
                                    <span v-else-if="days > 1" class="text-xs tabular-nums text-gray-500">{{ p.active_days }} of {{ days }} days</span>
                                </div>
                            </td>
                            <td class="hidden whitespace-nowrap px-3 py-3 text-right tabular-nums sm:table-cell">
                                <span class="font-medium text-gray-900">{{ count(p.prompts) }}</span>
                                <span
                                    v-if="change(p.prompts, p.prompts_before)"
                                    class="ml-1.5 inline-block w-12 text-left text-xs"
                                    :class="change(p.prompts, p.prompts_before) > 0 ? 'text-gray-500' : 'text-amber-700 dark:text-amber-400'"
                                    title="Compared with the period before"
                                >{{ change(p.prompts, p.prompts_before) > 0 ? '▲' : '▼' }} {{ Math.abs(change(p.prompts, p.prompts_before)) }}%</span>
                                <span v-else class="ml-1.5 inline-block w-12" />
                            </td>
                            <td class="hidden max-w-[12rem] px-3 py-3 md:table-cell">
                                <span class="block truncate" :class="p.main_project ? 'text-gray-700' : 'text-gray-400'" :title="p.main_tool ? `Mostly ${toolName(p.main_tool)}` : null">{{ p.main_project ?? (p.prompts ? 'General chat' : '—') }}</span>
                            </td>
                            <td class="hidden whitespace-nowrap py-3 pl-3 pr-5 text-right text-xs text-gray-500 lg:table-cell" :title="when(p.last_seen)">{{ p.last_seen ? ago(p.last_seen) : '—' }}</td>
                        </tr>
                        <tr v-if="!rows.length">
                            <td :colspan="columns.length" class="px-5 py-8 text-center text-sm text-gray-500">No one matches “{{ search }}”.</td>
                        </tr>
                    </tbody>
                </table>
            </Panel>

            <!-- Projects -->
            <Panel title="Projects" :subtitle="`Where the AI work went, ${periodName}`">
                <div v-if="!projects.length" class="px-5 py-10 text-center text-sm text-gray-500">No AI work in this period.</div>
                <ul v-else class="divide-y divide-gray-100">
                    <li v-for="p in projects" :key="p.repo ?? '-'">
                        <Link
                            :href="activityLink({ project: p.repo ?? '-' })"
                            class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 px-5 py-3 transition hover:bg-gray-50 sm:grid-cols-[minmax(0,12rem)_minmax(0,1fr)_8rem]"
                            :title="p.repo ?? 'Browser chats and tools run outside a repository'"
                        >
                            <span class="truncate text-sm" :class="p.repo ? 'font-medium text-gray-900' : 'italic text-gray-500'">{{ p.name ?? 'General chat (no project)' }}</span>

                            <!-- Share of the team's prompts -->
                            <div class="order-last col-span-2 flex items-center gap-3 sm:order-none sm:col-span-1">
                                <div class="h-2 flex-1 overflow-hidden rounded-full bg-gray-100">
                                    <div class="h-full rounded-full bg-indigo-600 dark:bg-indigo-400" :style="{ width: share(p) + '%' }" />
                                </div>
                                <span class="w-24 text-right text-xs tabular-nums text-gray-500">{{ share(p) }}% · {{ count(p.prompts) }}</span>
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
                        </Link>
                    </li>
                </ul>
            </Panel>

            <p class="px-1 text-xs leading-relaxed text-gray-500">
                This page shows how much AI is used, not how good anyone's work is: more prompts is not better work.
                <strong class="font-medium text-gray-700">Time with AI</strong> — {{ aiTimeDefinition }}
            </p>
        </div>
    </AuthenticatedLayout>
</template>
