<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { initials } from '@/Components/Usage/format';
import { Head, Link, router, useForm, usePage } from '@inertiajs/vue3';
import { computed, ref } from 'vue';
import { statusLabels } from '../../pm.js';

const props = defineProps({
    project: Object,
    projects: Array,
    sprints: Array,
    sprint: [Number, String],
    tasks: Array,
    statuses: Array,
    people: Array,
});

const me = computed(() => usePage().props.auth.user);
const activeTask = computed(() => usePage().props.pmActiveTask);
const columns = computed(() => props.statuses.map((s) => ({ status: s, tasks: props.tasks.filter((t) => t.status === s) })));

const switchProject = (id) => router.get(route('pm.board', id));
const switchSprint = (id) => router.get(route('pm.board', props.project.id), { sprint: id }, { preserveState: true });

// The card being saved fades until the server answers.
const saving = ref(null);
const move = (task, status) => {
    if (task.status !== status) {
        router.patch(route('pm.tasks.update', task.id), { status }, {
            preserveScroll: true,
            onStart: () => (saving.value = task.id),
            onFinish: () => (saving.value = null),
        });
    }
};
const start = (task) => router.post(route('pm.tasks.start', task.id), {}, { preserveScroll: true });
const canStart = (t) => t.status !== 'done' && (!t.assignee || t.assignee.id === me.value.id) && activeTask.value?.id !== t.id;

// Drag and drop: plain HTML5 events. The status select on each card does the
// same for keyboards and phones.
const dragging = ref(null);
const over = ref(null);
const drop = (status) => {
    const task = props.tasks.find((t) => t.id === dragging.value);
    if (task) move(task, status);
    dragging.value = over.value = null;
};

const form = useForm({ title: '', assignee_id: null, status: 'todo', sprint_id: null });
const add = () =>
    form
        .transform((d) => ({ ...d, sprint_id: d.status === 'backlog' || props.sprint === 'all' ? null : props.sprint }))
        .post(route('pm.tasks.store', props.project.id), { preserveScroll: true, onSuccess: () => form.reset('title') });
</script>

<template>
    <Head :title="`${project.key} board`" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-7xl space-y-5 px-4 py-8 sm:px-6 lg:px-8">
            <div class="flex flex-wrap items-end gap-3">
                <div class="me-auto">
                    <h1 class="text-xl font-semibold text-gray-900">{{ project.name }}</h1>
                    <p class="mt-1 text-sm text-gray-500">{{ tasks.length }} {{ tasks.length === 1 ? 'task' : 'tasks' }} on this board</p>
                </div>
                <label v-if="projects.length > 1" class="text-xs font-medium text-gray-500">
                    Project
                    <select :value="project.id" class="mt-1 block rounded-lg py-1.5 text-sm" @change="switchProject($event.target.value)">
                        <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.key }} · {{ p.name }}</option>
                    </select>
                </label>
                <label class="text-xs font-medium text-gray-500">
                    Sprint
                    <select :value="sprint" class="mt-1 block rounded-lg py-1.5 text-sm" @change="switchSprint($event.target.value)">
                        <option value="all">All tasks</option>
                        <option v-for="s in sprints" :key="s.id" :value="s.id">{{ s.name }}</option>
                    </select>
                </label>
            </div>

            <form class="flex flex-wrap items-center gap-2 rounded-xl border border-gray-200 bg-white p-3 shadow-sm" @submit.prevent="add">
                <label class="sr-only" for="new-task">New task title</label>
                <input id="new-task" v-model="form.title" type="text" required maxlength="255" placeholder="New task…" class="w-full min-w-0 rounded-lg text-sm sm:w-auto sm:flex-1" />
                <select v-model="form.assignee_id" aria-label="Assignee" class="rounded-lg text-sm">
                    <option :value="null">Unassigned</option>
                    <option v-for="p in people" :key="p.id" :value="p.id">{{ p.name }}</option>
                </select>
                <select v-model="form.status" aria-label="Status" class="rounded-lg text-sm">
                    <option v-for="s in statuses" :key="s" :value="s">{{ statusLabels[s] }}</option>
                </select>
                <button type="submit" :disabled="form.processing" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-indigo-50 shadow-sm hover:bg-indigo-500 disabled:opacity-50">Add</button>
                <p v-if="form.errors.title" class="w-full text-xs text-rose-600">{{ form.errors.title }}</p>
            </form>

            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
                <section
                    v-for="col in columns"
                    :key="col.status"
                    class="flex min-h-[8rem] flex-col rounded-xl bg-gray-100 p-2 transition"
                    :class="over === col.status ? 'ring-2 ring-indigo-400' : ''"
                    :aria-label="statusLabels[col.status]"
                    @dragover.prevent="over = col.status"
                    @dragleave="over = over === col.status ? null : over"
                    @drop.prevent="drop(col.status)"
                >
                    <h2 class="flex items-center justify-between px-2 pb-2 pt-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                        {{ statusLabels[col.status] }}
                        <span class="font-normal">{{ col.tasks.length }}</span>
                    </h2>

                    <p v-if="!col.tasks.length" class="px-2 py-4 text-center text-xs text-gray-400">Nothing here</p>

                    <ul class="space-y-2">
                        <li
                            v-for="t in col.tasks"
                            :key="t.id"
                            draggable="true"
                            class="cursor-grab rounded-lg border border-gray-200 bg-white p-3 shadow-sm active:cursor-grabbing"
                            :class="[dragging === t.id || saving === t.id ? 'opacity-50' : '', activeTask?.id === t.id ? 'ring-2 ring-emerald-400' : '']"
                            @dragstart="dragging = t.id"
                            @dragend="dragging = over = null"
                        >
                            <div class="flex items-center justify-between gap-2 text-xs text-gray-500">
                                <span class="font-mono">{{ t.key }}</span>
                                <span
                                    v-if="t.ai_sessions"
                                    class="rounded-full bg-indigo-50 px-2 py-0.5 font-medium text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300"
                                    :title="`${t.ai_sessions} AI ${t.ai_sessions === 1 ? 'session' : 'sessions'} linked`"
                                >AI {{ t.ai_sessions }}</span>
                            </div>
                            <Link :href="route('pm.tasks.show', t.id)" class="mt-1 block text-sm font-medium text-gray-900 hover:underline">{{ t.title }}</Link>
                            <div class="mt-3 flex flex-wrap items-center gap-2">
                                <span
                                    v-if="t.assignee"
                                    class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-gray-200 text-[10px] font-semibold text-gray-700"
                                    :title="t.assignee.name"
                                >{{ initials(t.assignee.name) }}</span>
                                <span v-else class="text-xs text-gray-400">Unassigned</span>
                                <button
                                    v-if="canStart(t)"
                                    type="button"
                                    class="text-xs font-medium text-emerald-700 hover:underline dark:text-emerald-400"
                                    @click="start(t)"
                                >Start</button>
                                <select
                                    :value="t.status"
                                    :aria-label="`Move ${t.key}`"
                                    class="ms-auto rounded-md border-gray-200 py-0.5 pl-1.5 pr-7 text-xs text-gray-600"
                                    @change="move(t, $event.target.value)"
                                >
                                    <option v-for="s in statuses" :key="s" :value="s">{{ statusLabels[s] }}</option>
                                </select>
                            </div>
                        </li>
                    </ul>
                </section>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
