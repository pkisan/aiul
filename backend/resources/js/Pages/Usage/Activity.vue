<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Pagination from '@/Components/Usage/Pagination.vue';
import Tag from '@/Components/Usage/Tag.vue';
import SearchSelect from '@/Components/SearchSelect.vue';
import { ago, count, duration, initials, toolName, when } from '@/Components/Usage/format';
import { Head, Link, router } from '@inertiajs/vue3';
import { computed } from 'vue';

const props = defineProps({
    filters: Object,
    summary: Object,
    list: Object,
    options: Object,
    aiTimeDefinition: String,
    canViewRaw: Boolean,
});

const periods = [
    { value: 1, label: 'Today' },
    { value: 7, label: '7 days' },
    { value: 30, label: '30 days' },
    { value: 90, label: '90 days' },
    { value: 365, label: '1 year' },
];

// Every change is a visit with the filters in the URL, so any view can be
// bookmarked or shared. Changing a filter goes back to page 1.
function go(changes) {
    const next = { ...props.filters, ...changes };
    const query = Object.fromEntries(
        Object.entries(next).filter(([k, v]) => v !== null && v !== '' && !(k === 'days' && v === 30) && !(k === 'view' && v === 'prompts')),
    );
    router.get(route('usage.activity'), query, { preserveScroll: true, preserveState: true });
}

const filtered = computed(() => props.filters.person || props.filters.tool || props.filters.project);
const personName = (id) => props.options.people.find((p) => p.id === id)?.name;

</script>

<template>
    <Head title="Activity" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-7xl space-y-5 px-4 py-6 sm:px-6 lg:px-8">
            <!-- Title and the period in one line of plain numbers -->
            <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
                <div>
                    <h1 class="text-2xl font-semibold tracking-tight text-gray-900">Activity</h1>
                    <p class="mt-1 text-sm text-gray-500">
                        <span class="font-medium text-gray-900">{{ count(summary.prompts) }}</span> prompts
                        from <span class="font-medium text-gray-900">{{ count(summary.people) }}</span>
                        {{ summary.people === 1 ? 'person' : 'people' }} ·
                        <span class="font-medium text-gray-900">{{ count(summary.sessions) }}</span> sessions ·
                        <span class="font-medium text-gray-900" :title="aiTimeDefinition">{{ duration(summary.ai_seconds) }}</span>
                        AI time
                    </p>
                </div>

                <div class="inline-flex self-start rounded-lg border border-gray-200 bg-white p-0.5 shadow-sm" role="group" aria-label="Period">
                    <button
                        v-for="p in periods"
                        :key="p.value"
                        type="button"
                        class="rounded-md px-3 py-1.5 text-sm font-medium transition"
                        :class="filters.days === p.value ? 'bg-gray-900 text-white' : 'text-gray-600 hover:text-gray-900'"
                        :aria-pressed="filters.days === p.value"
                        @click="go({ days: p.value })"
                    >{{ p.label }}</button>
                </div>
            </div>

            <!-- Filters -->
            <div class="flex flex-wrap items-center gap-2">
                <SearchSelect
                    class="w-44"
                    label="Person"
                    :model-value="filters.person ?? ''"
                    :options="[{ value: '', label: 'Everyone' }, ...options.people.map((p) => ({ value: p.id, label: p.name }))]"
                    @update:model-value="(v) => go({ person: v || null })"
                />
                <SearchSelect
                    class="w-40"
                    label="Tool"
                    :model-value="filters.tool ?? ''"
                    :options="[{ value: '', label: 'All tools' }, ...options.tools.map((t) => ({ value: t, label: toolName(t) }))]"
                    @update:model-value="(v) => go({ tool: v || null })"
                />
                <SearchSelect
                    class="w-48 max-w-[16rem]"
                    label="Project"
                    :model-value="filters.project ?? ''"
                    :options="[
                        { value: '', label: 'All projects' },
                        { value: '-', label: 'Outside a project' },
                        ...options.projects.map((p) => ({ value: p.repo, label: p.name, title: p.repo })),
                    ]"
                    @update:model-value="(v) => go({ project: v || null })"
                />
                <button
                    v-if="filtered"
                    type="button"
                    class="rounded-lg px-3 py-1.5 text-sm text-gray-500 hover:bg-gray-100 hover:text-gray-900"
                    @click="go({ person: null, tool: null, project: null })"
                >Clear filters</button>
                <Link
                    v-if="filters.project && filters.project !== '-'"
                    :href="route('usage.project', { repo: filters.project, days: filters.days })"
                    class="px-3 py-1.5 text-sm text-indigo-600 hover:underline dark:text-indigo-400"
                >Every interaction in this project →</Link>
            </div>

                            <!-- The list: what people asked, or the sessions it happened in -->
                <section class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
                    <div class="flex items-center justify-between gap-4 border-b border-gray-100 px-5">
                        <nav class="-mb-px flex gap-5" aria-label="List">
                            <button
                                v-for="tab in [{ v: 'prompts', l: 'Recent prompts' }, { v: 'sessions', l: 'Sessions' }]"
                                :key="tab.v"
                                type="button"
                                class="border-b-2 py-3.5 text-sm font-medium transition"
                                :class="filters.view === tab.v ? 'border-indigo-600 text-gray-900' : 'border-transparent text-gray-500 hover:text-gray-900'"
                                :aria-current="filters.view === tab.v ? 'page' : undefined"
                                @click="go({ view: tab.v })"
                            >{{ tab.l }}</button>
                        </nav>
                        <span v-if="filters.person" class="truncate text-xs text-gray-500">{{ personName(filters.person) }}</span>
                    </div>

                    <!-- Prompts -->
                    <ul v-if="filters.view === 'prompts'" class="divide-y divide-gray-100">
                        <li v-for="p in list.data" :key="p.id">
                            <Link
                                :href="route('usage.session', p.session_id) + '#i-' + p.id"
                                class="flex gap-3 px-5 py-3.5 transition hover:bg-gray-50"
                            >
                                <span
                                    class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-xs font-semibold text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300"
                                    aria-hidden="true"
                                >{{ initials(p.person ?? p.account) }}</span>
                                <div class="min-w-0 flex-1">
                                    <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                                        <span class="font-medium text-gray-900">{{ p.person ?? 'Unassigned device' }}</span>
                                        <Tag :label="toolName(p.tool)" tone="blue" />
                                        <span v-if="p.project" class="truncate text-gray-500">
                                            {{ p.project }}<template v-if="p.branch"> · {{ p.branch }}</template>
                                        </span>
                                        <span class="ml-auto shrink-0 text-xs text-gray-400" :title="when(p.occurred_at)">{{ ago(p.occurred_at) }}</span>
                                    </div>
                                    <p v-if="p.preview" class="mt-1 line-clamp-2 text-sm text-gray-600">{{ p.preview }}</p>
                                    <p v-else class="mt-1 text-xs text-gray-400">
                                        {{ count(p.prompt_chars) }} characters<template v-if="p.model"> · {{ p.model }}</template>
                                        <template v-if="p.account && p.account !== p.person"> · as {{ p.account }}</template>
                                    </p>
                                </div>
                            </Link>
                        </li>
                    </ul>

                    <!-- Sessions -->
                    <ul v-else class="divide-y divide-gray-100">
                        <li v-for="s in list.data" :key="s.id">
                            <Link :href="route('usage.session', s.id)" class="flex items-center gap-4 px-5 py-3.5 transition hover:bg-gray-50">
                                <div class="min-w-0 flex-1">
                                    <div class="flex flex-wrap items-center gap-2 text-sm">
                                        <span class="truncate font-medium text-gray-900">{{ s.project ?? toolName(s.tool) }}</span>
                                        <Tag v-if="s.project" :label="toolName(s.tool)" tone="blue" />
                                        <Tag v-if="s.branch" :label="s.branch" />
                                    </div>
                                    <div class="mt-0.5 truncate text-xs text-gray-500">
                                        {{ s.person ?? s.account ?? 'Unassigned device' }} ·
                                        <span :title="when(s.ended_at)">{{ ago(s.ended_at ?? s.started_at) }}</span>
                                    </div>
                                </div>
                                <div class="shrink-0 text-right">
                                    <div class="text-sm font-medium tabular-nums text-gray-900">{{ s.human_prompts }}</div>
                                    <div class="text-xs text-gray-400">prompts</div>
                                </div>
                                <div class="hidden w-16 shrink-0 text-right sm:block">
                                    <div class="text-sm tabular-nums text-gray-900">{{ duration(s.seconds) }}</div>
                                    <div class="text-xs text-gray-400">AI time</div>
                                </div>
                            </Link>
                        </li>
                    </ul>

                    <div v-if="!list.data.length" class="px-5 py-16 text-center">
                        <p class="text-sm font-medium text-gray-900">Nothing here yet</p>
                        <p class="mt-1 text-sm text-gray-500">
                            {{ filtered ? 'No activity matches these filters in this period.' : 'No AI activity captured in this period.' }}
                        </p>
                    </div>

                    <Pagination :page="list" :noun="filters.view" />
                </section>

                <p class="px-1 text-xs leading-relaxed text-gray-500">
                    <strong class="font-medium text-gray-700">AI time</strong> — {{ aiTimeDefinition }}
                </p>
        </div>
    </AuthenticatedLayout>
</template>
