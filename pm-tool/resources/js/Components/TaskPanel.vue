<script setup>
// Everything about one task, shown in the board's slide-over. The shell
// (position, animation, closing) is the board's; this is the content.
import Panel from '@/Components/Usage/Panel.vue';
import { when } from '@/Components/Usage/format';
import { Link, router, useForm, usePage } from '@inertiajs/vue3';
import { computed, onMounted, ref, watch } from 'vue';
import { statusLabels } from '../pm.js';
import Trail from './Trail.vue';

const props = defineProps({ task: Object, trail: Object, people: Array, statuses: Array, projects: Array });
const emit = defineEmits(['close']);

const me = computed(() => usePage().props.auth.user);
const isActive = computed(() => usePage().props.pmActiveTask?.id === props.task.id);
const canStart = computed(() => props.task.status !== 'done' && (!props.task.assignee || props.task.assignee.id === me.value.id));
const project = computed(() => props.projects.find((p) => p.id === props.task.project_id));
const others = computed(() => props.projects.filter((p) => p.id !== props.task.project_id));

// Every change keeps the board and this panel as they are (no remount, no scroll jump).
const keep = { preserveScroll: true, preserveState: true };
const save = (changes) => router.patch(route('pm.tasks.update', props.task.id), changes, keep);

// Title and description save with the button. Unsaved text survives closing the
// panel or reloading the page: it is kept in this browser until saved.
const draftKey = `pm:draft:${props.task.id}`;
const readDraft = () => {
    try {
        return JSON.parse(localStorage.getItem(draftKey) ?? 'null');
    } catch {
        return null;
    }
};
const draft = ref(readDraft());
const form = useForm({ title: props.task.title, description: props.task.description ?? '' });
if (draft.value) {
    form.title = draft.value.title;
    form.description = draft.value.description;
}
watch(
    () => [form.title, form.description],
    () => {
        try {
            form.isDirty ? localStorage.setItem(draftKey, JSON.stringify({ title: form.title, description: form.description })) : localStorage.removeItem(draftKey);
        } catch {
            // private mode or storage blocked: the draft just is not kept
        }
    },
);
const saveText = () =>
    form.patch(route('pm.tasks.update', props.task.id), {
        ...keep,
        onSuccess: () => {
            form.defaults();
            draft.value = null;
            try {
                localStorage.removeItem(draftKey);
            } catch {}
        },
    });

// A branch name with the task key in it: AI work on that branch links to
// this task by itself ("aay-4-task-ai-trail-timeline").
const branch = computed(() =>
    `${props.task.key.toLowerCase()}-${props.task.title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 40).replace(/-$/, '')}`,
);
const copied = ref(false);
const copyBranch = () =>
    navigator.clipboard?.writeText(branch.value).then(() => {
        copied.value = true;
        setTimeout(() => (copied.value = false), 1500);
    });

const person = (id) => props.people.find((p) => p.id === Number(id))?.name ?? 'nobody';
const describe = (e) =>
    ({
        status: () => `moved it ${e.from ? `from ${statusLabels[e.from]} ` : ''}to ${statusLabels[e.to]}`,
        assignee: () => `assigned it to ${e.to ? person(e.to) : 'nobody'}`,
        project: () => `moved it from ${e.from} to ${e.to}`,
    })[e.field]?.() ?? `changed ${e.field}`;

// Moving changes the key and clears the sprint; AI sessions stay with the task.
const move = (select) => {
    const target = others.value.find((p) => p.id === Number(select.value));
    const sure =
        target &&
        confirm(
            `Move ${props.task.key} to ${target.name}? It gets a ${target.key} key and leaves its sprint. ` +
                `Its AI sessions stay linked, and branches named ${props.task.key} still work.`,
        );
    if (sure) {
        router.post(route('pm.tasks.move', props.task.id), { project_id: target.id }, keep);
    } else {
        select.value = props.task.project_id;
    }
};

// Keyboard and screen-reader users land on the title when the panel opens.
const heading = ref(null);
onMounted(() => heading.value?.focus({ preventScroll: true }));
</script>

<template>
    <div class="space-y-5 px-5 pb-8 pt-4 sm:px-6">
        <div class="flex items-start gap-3">
            <h2 id="task-panel-title" ref="heading" tabindex="-1" class="me-auto text-lg font-semibold text-gray-900 outline-none">
                <span class="font-mono text-sm font-normal text-gray-500">{{ task.key }}</span>
                {{ task.title }}
            </h2>
            <button type="button" class="-mr-1 rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900" aria-label="Close task" @click="emit('close')">
                <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
            <Link
                v-if="isActive"
                :href="route('pm.work.stop')"
                method="post"
                as="button"
                preserve-scroll
                preserve-state
                class="rounded-lg border border-emerald-300 px-3 py-1.5 text-sm font-medium text-emerald-800 hover:bg-emerald-50 dark:border-emerald-800 dark:text-emerald-300 dark:hover:bg-emerald-950"
            >Stop working</Link>
            <Link
                v-else-if="canStart"
                :href="route('pm.tasks.start', task.id)"
                method="post"
                as="button"
                preserve-scroll
                preserve-state
                class="rounded-lg bg-emerald-600 px-3 py-1.5 text-sm font-medium text-emerald-50 shadow-sm hover:bg-emerald-500"
            >Start working</Link>
            <span class="text-xs text-gray-500">Branch:</span>
            <code class="min-w-0 truncate rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-700">{{ branch }}</code>
            <button type="button" class="text-xs font-medium text-indigo-600 hover:underline dark:text-indigo-400" @click="copyBranch">{{ copied ? 'Copied' : 'Copy' }}</button>
        </div>
        <p v-if="isActive" class="-mt-2 text-sm text-emerald-700 dark:text-emerald-400">You are working on this. AI sessions you start now count for this task.</p>

        <div class="grid gap-3 sm:grid-cols-2">
            <label class="text-xs font-medium text-gray-500">
                Status
                <select :value="task.status" class="mt-1 block w-full rounded-lg text-sm" @change="save({ status: $event.target.value })">
                    <option v-for="s in statuses" :key="s" :value="s">{{ statusLabels[s] }}</option>
                </select>
            </label>
            <label class="text-xs font-medium text-gray-500">
                Assignee
                <select :value="task.assignee?.id ?? ''" class="mt-1 block w-full rounded-lg text-sm" @change="save({ assignee_id: $event.target.value || null })">
                    <option value="">Unassigned</option>
                    <option v-for="p in people" :key="p.id" :value="p.id">{{ p.name }}</option>
                </select>
            </label>
            <label class="text-xs font-medium text-gray-500">
                Sprint
                <select :value="task.sprint_id ?? ''" class="mt-1 block w-full rounded-lg text-sm" @change="save({ sprint_id: $event.target.value || null })">
                    <option value="">No sprint</option>
                    <option v-for="s in task.sprints" :key="s.id" :value="s.id">{{ s.name }}</option>
                </select>
            </label>
            <label class="text-xs font-medium text-gray-500">
                Project
                <select :value="task.project_id" :disabled="!others.length" class="mt-1 block w-full rounded-lg text-sm disabled:opacity-60" @change="move($event.target)">
                    <option :value="task.project_id">{{ project?.key }} · {{ project?.name }}</option>
                    <option v-for="p in others" :key="p.id" :value="p.id">Move to {{ p.key }} · {{ p.name }}</option>
                </select>
            </label>
        </div>

        <Trail :trail="trail" :task-key="task.key" />

        <Panel title="Details">
            <form class="space-y-4 px-5 py-4" @submit.prevent="saveText">
                <p v-if="draft" class="text-xs text-amber-800 dark:text-amber-300">Unsaved changes from earlier were restored.</p>
                <label class="block text-xs font-medium text-gray-500">
                    Title
                    <input v-model="form.title" type="text" required maxlength="255" class="mt-1 block w-full rounded-lg text-sm" />
                    <span v-if="form.errors.title" class="mt-1 block text-rose-600">{{ form.errors.title }}</span>
                </label>
                <label class="block text-xs font-medium text-gray-500">
                    Description
                    <textarea v-model="form.description" rows="5" maxlength="20000" class="mt-1 block w-full rounded-lg text-sm"></textarea>
                </label>
                <button
                    type="submit"
                    :disabled="form.processing || !form.isDirty"
                    class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-indigo-50 shadow-sm hover:bg-indigo-500 disabled:opacity-50"
                >Save</button>
            </form>
        </Panel>

        <Panel title="History">
            <ul class="divide-y divide-gray-100 text-sm">
                <li v-for="e in task.events" :key="e.id" class="flex flex-wrap justify-between gap-2 px-5 py-2.5">
                    <span class="text-gray-700"><span class="font-medium text-gray-900">{{ e.by ?? 'Someone' }}</span> {{ describe(e) }}</span>
                    <span class="text-xs text-gray-500">{{ when(e.at) }}</span>
                </li>
                <li class="flex justify-between gap-2 px-5 py-2.5">
                    <span class="text-gray-700">Created</span>
                    <span class="text-xs text-gray-500">{{ when(task.created_at) }}</span>
                </li>
            </ul>
        </Panel>
    </div>
</template>
