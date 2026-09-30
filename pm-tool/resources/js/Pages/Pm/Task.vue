<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { when } from '@/Components/Usage/format';
import { Head, Link, router, useForm, usePage } from '@inertiajs/vue3';
import { computed, ref } from 'vue';
import Trail from '../../Components/Trail.vue';
import { statusLabels } from '../../pm.js';

const props = defineProps({ task: Object, project: Object, events: Array, sprints: Array, statuses: Array, people: Array, trail: Object });

const me = computed(() => usePage().props.auth.user);
const isActive = computed(() => usePage().props.pmActiveTask?.id === props.task.id);
const canStart = computed(() => props.task.status !== 'done' && (!props.task.assignee || props.task.assignee.id === me.value.id));

// Selects save on change; title and description save with the button.
const save = (changes) => router.patch(route('pm.tasks.update', props.task.id), changes, { preserveScroll: true });
const form = useForm({ title: props.task.title, description: props.task.description ?? '' });
const saveText = () => form.patch(route('pm.tasks.update', props.task.id), { preserveScroll: true });

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
    e.field === 'status'
        ? `moved it ${e.from ? `from ${statusLabels[e.from]} ` : ''}to ${statusLabels[e.to]}`
        : `assigned it to ${e.to ? person(e.to) : 'nobody'}`;
</script>

<template>
    <Head :title="`${task.key} ${task.title}`" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-4xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <div>
                <Link :href="route('pm.board', project.id)" class="text-sm text-gray-500 hover:underline">← {{ project.name }} board</Link>
                <div class="mt-2 flex flex-wrap items-start gap-3">
                    <h1 class="me-auto text-xl font-semibold text-gray-900">
                        <span class="font-mono text-base font-normal text-gray-500">{{ task.key }}</span>
                        {{ task.title }}
                    </h1>
                    <Link
                        v-if="isActive"
                        :href="route('pm.work.stop')"
                        method="post"
                        as="button"
                        preserve-scroll
                        class="rounded-lg border border-emerald-300 px-4 py-2 text-sm font-medium text-emerald-800 hover:bg-emerald-50 dark:border-emerald-800 dark:text-emerald-300 dark:hover:bg-emerald-950"
                    >Stop working</Link>
                    <Link
                        v-else-if="canStart"
                        :href="route('pm.tasks.start', task.id)"
                        method="post"
                        as="button"
                        preserve-scroll
                        class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-emerald-50 shadow-sm hover:bg-emerald-500"
                    >Start working</Link>
                </div>
                <p class="mt-2 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                    Branch:
                    <code class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-gray-700">{{ branch }}</code>
                    <button type="button" class="font-medium text-indigo-600 hover:underline dark:text-indigo-400" @click="copyBranch">{{ copied ? 'Copied' : 'Copy branch name' }}</button>
                </p>
                <p v-if="isActive" class="mt-1 text-sm text-emerald-700 dark:text-emerald-400">
                    You are working on this. AI sessions you start now count for this task.
                </p>
            </div>

            <div class="grid gap-4 sm:grid-cols-3">
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
                        <option v-for="s in sprints" :key="s.id" :value="s.id">{{ s.name }}</option>
                    </select>
                </label>
            </div>

            <Trail :trail="trail" :task-key="task.key" />

            <Panel title="Details">
                <form class="space-y-4 px-5 py-4" @submit.prevent="saveText">
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
                    <li v-for="e in events" :key="e.id" class="flex flex-wrap justify-between gap-2 px-5 py-2.5">
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
    </AuthenticatedLayout>
</template>
