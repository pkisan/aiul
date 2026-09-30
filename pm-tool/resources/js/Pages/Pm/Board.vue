<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { initials } from '@/Components/Usage/format';
import { Head, Link, router, useForm, usePage } from '@inertiajs/vue3';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import TaskPanel from '../../Components/TaskPanel.vue';
import { statusLabels } from '../../pm.js';

const props = defineProps({
    project: Object,
    projects: Array,
    sprints: Array,
    sprint: [Number, String],
    tasks: Array,
    statuses: Array,
    people: Array,
    // The task open in the slide-over (/pm/tasks/{id}), and its AI trail; null when closed.
    task: Object,
    trail: Object,
});

// ---- Slide-over. Opening or closing only reloads `task` and `trail`: the
// board keeps its data, scroll and filters, and the URL is the task's own.
const only = { only: ['task', 'trail'], preserveState: true, preserveScroll: true };
const taskHref = (t) => route('pm.tasks.show', { task: t.id, sprint: props.sprint });
let opener = null; // the card link that opened the panel, to give focus back
const close = () => {
    if (!props.task) return;
    router.visit(route('pm.board', { project: props.project.id, sprint: props.sprint }), {
        ...only,
        onSuccess: () => nextTick(() => opener?.focus()),
    });
};
const onKey = (e) => e.key === 'Escape' && props.task && !e.defaultPrevented && close();
onMounted(() => document.addEventListener('keydown', onKey));
// The open task's card scrolls into view in the side-scrolling columns.
const showSelected = () => nextTick(() => document.querySelector('[aria-current="true"]')?.closest('li')?.scrollIntoView({ block: 'nearest', inline: 'nearest' }));
onMounted(showSelected);
watch(() => props.task?.id, showSelected);
onBeforeUnmount(() => document.removeEventListener('keydown', onKey));
// A click on empty board space closes the panel; cards and controls do not.
const onBoardClick = (e) => {
    if (props.task && !e.target.closest('li, a, button, select, input, textarea, label, form')) close();
};

const me = computed(() => usePage().props.auth.user);
const activeTask = computed(() => usePage().props.pmActiveTask);
const columns = computed(() => props.statuses.map((s) => ({ status: s, tasks: props.tasks.filter((t) => t.status === s) })));

const switchProject = (id) => router.get(route('pm.board', id));
// A link to this board with the chosen sprint, e.g. /pm/projects/10/board?sprint=3.
// The sprint is always written out: a link with no ?sprint opens whichever
// sprint is running on the day it is clicked.
const sprintName = computed(() => (props.sprint === 'all' ? 'all tasks' : props.sprints.find((s) => s.id === props.sprint)?.name ?? 'this sprint'));
const linkCopied = ref(false);
const copySprintLink = () =>
    navigator.clipboard?.writeText(route('pm.board', { project: props.project.id, sprint: props.sprint })).then(() => {
        linkCopied.value = true;
        setTimeout(() => (linkCopied.value = false), 1500);
    });
const switchSprint = (id) => router.get(route('pm.board', props.project.id), { sprint: id }, { preserveState: true });

// The card being saved fades until the server answers.
const saving = ref(null);
const move = (task, status) => {
    if (task.status !== status) {
        router.patch(route('pm.tasks.update', task.id), { status }, {
            preserveScroll: true,
            preserveState: true,
            onStart: () => (saving.value = task.id),
            onFinish: () => (saving.value = null),
        });
    }
};
const start = (task) => router.post(route('pm.tasks.start', task.id), {}, { preserveScroll: true, preserveState: true });
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
    <Head :title="task ? `${task.key} ${task.title}` : `${project.key} board`" />

    <AuthenticatedLayout>
        <!-- With a task open (from 1024px) the board shrinks to the space left of
             the panel and its columns scroll sideways, so every card can still
             be clicked to swap the task. -->
        <div
            class="space-y-5 px-4 py-8 sm:px-6 lg:px-8"
            :class="task ? 'mx-auto max-w-7xl lg:mx-0 lg:mr-[60vw] lg:max-w-none xl:mr-[720px]' : 'mx-auto max-w-7xl'"
            @click="onBoardClick"
        >
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
                <button
                    type="button"
                    class="rounded-lg px-2 py-1.5 text-sm font-medium text-indigo-600 hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-indigo-950"
                    :title="`Copy a link to this board showing ${sprintName}`"
                    @click="copySprintLink"
                >{{ linkCopied ? 'Copied' : 'Copy link' }}</button>
            </div>

            <form class="flex flex-wrap items-center gap-2 rounded-xl border border-gray-200 bg-white p-3 shadow-sm" @submit.prevent="add">
                <label class="sr-only" for="new-task">New task title</label>
                <input id="new-task" v-model="form.title" type="text" required maxlength="255" placeholder="New task…" class="w-full min-w-0 rounded-lg text-sm" :class="task ? '' : 'sm:w-auto sm:flex-1'" />
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

            <div
                class="grid gap-4 sm:grid-cols-2"
                :class="task ? 'lg:auto-cols-[15rem] lg:grid-flow-col lg:grid-cols-none lg:overflow-x-auto lg:pb-2' : 'lg:grid-cols-5'"
            >
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
                            :class="[
                                dragging === t.id || saving === t.id ? 'opacity-50' : '',
                                task?.id === t.id ? 'ring-2 ring-indigo-500' : activeTask?.id === t.id ? 'ring-2 ring-emerald-400' : '',
                            ]"
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
                            <Link
                                :href="taskHref(t)"
                                :only="['task', 'trail']"
                                preserve-state
                                preserve-scroll
                                class="mt-1 block text-sm font-medium text-gray-900 hover:underline"
                                :aria-current="task?.id === t.id ? 'true' : undefined"
                                @click="opener = $event.currentTarget"
                            >{{ t.title }}</Link>
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

        <!-- Task slide-over: right side. Below 1024px it covers the screen over a
             backdrop; from 1024px the board stays usable beside it. -->
        <Transition
            enter-active-class="transition-opacity duration-200 motion-reduce:transition-none"
            leave-active-class="transition-opacity duration-150 motion-reduce:transition-none"
            enter-from-class="opacity-0"
            leave-to-class="opacity-0"
        >
            <div v-if="task" class="fixed inset-0 z-40 bg-gray-900/40 lg:hidden" aria-hidden="true" @click="close"></div>
        </Transition>
        <Transition
            enter-active-class="transition-transform duration-200 ease-out motion-reduce:transition-none"
            leave-active-class="transition-transform duration-150 ease-in motion-reduce:transition-none"
            enter-from-class="translate-x-full"
            leave-to-class="translate-x-full"
        >
            <aside
                v-if="task"
                class="fixed inset-0 z-50 overflow-y-auto border-gray-200 bg-gray-50 shadow-2xl lg:inset-auto lg:bottom-0 lg:right-0 lg:top-16 lg:z-20 lg:w-[60vw] lg:border-l xl:w-[720px]"
                role="dialog"
                aria-labelledby="task-panel-title"
            >
                <TaskPanel :key="task.id" :task="task" :trail="trail" :people="people" :statuses="statuses" :projects="projects" @close="close" />
            </aside>
        </Transition>
    </AuthenticatedLayout>
</template>
