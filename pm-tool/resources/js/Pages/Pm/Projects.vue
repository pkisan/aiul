<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { Head, Link, useForm } from '@inertiajs/vue3';
import { ref } from 'vue';
import { day } from '../../pm.js';

defineProps({ projects: Array, canManage: Boolean });

const create = useForm({ name: '', key: '', remotes: '' });
const add = () => create.post(route('pm.projects.store'));

// One edit form and one sprint form, opened under the project they are for.
const editing = ref(null);
const edit = useForm({ name: '', remotes: '' });
const openEdit = (p) => {
    edit.clearErrors();
    edit.name = p.name;
    edit.remotes = p.remotes.join('\n');
    editing.value = editing.value === p.id ? null : p.id;
};
const saveEdit = (p) => edit.patch(route('pm.projects.update', p.id), { preserveScroll: true, onSuccess: () => (editing.value = null) });

const sprintFor = ref(null);
const sprint = useForm({ name: '', start_date: '', end_date: '' });
const openSprint = (p) => {
    sprint.reset();
    sprint.clearErrors();
    sprintFor.value = sprintFor.value === p.id ? null : p.id;
};
const saveSprint = (p) => sprint.post(route('pm.sprints.store', p.id), { preserveScroll: true, onSuccess: () => (sprintFor.value = null) });
</script>

<template>
    <Head title="Projects" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <div>
                <h1 class="text-xl font-semibold text-gray-900">Projects</h1>
                <p class="mt-1 text-sm text-gray-500">
                    A project's git remotes tell us which AI work belongs to it: sessions in those repositories count for its tasks.
                </p>
            </div>

            <Panel v-if="canManage" title="New project">
                <form class="grid gap-4 px-5 py-4 sm:grid-cols-[1fr_8rem]" @submit.prevent="add">
                    <label class="text-xs font-medium text-gray-500">
                        Name
                        <input v-model="create.name" type="text" required maxlength="255" class="mt-1 block w-full rounded-lg text-sm" />
                        <span v-if="create.errors.name" class="mt-1 block text-rose-600">{{ create.errors.name }}</span>
                    </label>
                    <label class="text-xs font-medium text-gray-500">
                        Key
                        <input
                            v-model="create.key"
                            type="text"
                            required
                            maxlength="6"
                            placeholder="AAY"
                            class="mt-1 block w-full rounded-lg font-mono text-sm uppercase"
                            @input="create.key = create.key.toUpperCase()"
                        />
                        <span v-if="create.errors.key" class="mt-1 block text-rose-600">{{ create.errors.key }}</span>
                    </label>
                    <label class="text-xs font-medium text-gray-500 sm:col-span-2">
                        Git remotes, one per line (optional)
                        <textarea v-model="create.remotes" rows="2" placeholder="github.com/your-org/your-repo" class="mt-1 block w-full rounded-lg font-mono text-sm"></textarea>
                        <span v-if="create.errors.remotes" class="mt-1 block text-rose-600">{{ create.errors.remotes }}</span>
                    </label>
                    <div>
                        <button type="submit" :disabled="create.processing" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-indigo-50 shadow-sm hover:bg-indigo-500 disabled:opacity-50">Create project</button>
                    </div>
                </form>
            </Panel>

            <p v-if="!projects.length" class="rounded-xl border border-dashed border-gray-300 px-5 py-10 text-center text-sm text-gray-500">
                No projects yet.{{ canManage ? ' Create the first one above.' : ' A manager creates them.' }}
            </p>

            <Panel v-for="p in projects" :key="p.id" :title="`${p.key} · ${p.name}`" :subtitle="`${p.open} open of ${p.tasks} tasks`">
                <template #actions>
                    <div class="flex gap-3">
                        <button v-if="canManage" type="button" class="font-medium text-gray-600 hover:underline" @click="openEdit(p)">Edit</button>
                        <button v-if="canManage" type="button" class="font-medium text-gray-600 hover:underline" @click="openSprint(p)">New sprint</button>
                        <Link :href="route('pm.board', p.id)" class="font-medium text-indigo-600 hover:underline dark:text-indigo-400">Open board</Link>
                    </div>
                </template>

                <div class="grid gap-4 px-5 py-4 text-sm sm:grid-cols-2">
                    <div>
                        <h4 class="text-xs font-medium uppercase tracking-wide text-gray-500">Git remotes</h4>
                        <ul v-if="p.remotes.length" class="mt-1 space-y-0.5 font-mono text-xs text-gray-700">
                            <li v-for="r in p.remotes" :key="r" class="break-all">{{ r }}</li>
                        </ul>
                        <p v-else class="mt-1 text-gray-500">None yet: AI work cannot be matched to this project by repository.</p>
                    </div>
                    <div>
                        <h4 class="text-xs font-medium uppercase tracking-wide text-gray-500">Sprints</h4>
                        <ul v-if="p.sprints.length" class="mt-1 space-y-0.5 text-gray-700">
                            <li v-for="s in p.sprints" :key="s.id">{{ s.name }} <span class="text-xs text-gray-500">{{ day(s.start_date) }} – {{ day(s.end_date) }}</span></li>
                        </ul>
                        <p v-else class="mt-1 text-gray-500">None yet.</p>
                    </div>
                </div>

                <form v-if="editing === p.id" class="grid gap-4 border-t border-gray-100 px-5 py-4" @submit.prevent="saveEdit(p)">
                    <label class="text-xs font-medium text-gray-500">
                        Name
                        <input v-model="edit.name" type="text" required maxlength="255" class="mt-1 block w-full rounded-lg text-sm" />
                    </label>
                    <label class="text-xs font-medium text-gray-500">
                        Git remotes, one per line
                        <textarea v-model="edit.remotes" rows="3" class="mt-1 block w-full rounded-lg font-mono text-sm"></textarea>
                        <span v-if="edit.errors.remotes" class="mt-1 block text-rose-600">{{ edit.errors.remotes }}</span>
                    </label>
                    <div>
                        <button type="submit" :disabled="edit.processing" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-indigo-50 shadow-sm hover:bg-indigo-500 disabled:opacity-50">Save</button>
                    </div>
                </form>

                <form v-if="sprintFor === p.id" class="grid gap-4 border-t border-gray-100 px-5 py-4 sm:grid-cols-[1fr_10rem_10rem_auto] sm:items-start" @submit.prevent="saveSprint(p)">
                    <label class="text-xs font-medium text-gray-500">
                        Sprint name
                        <input v-model="sprint.name" type="text" required maxlength="255" class="mt-1 block w-full rounded-lg text-sm" />
                    </label>
                    <label class="text-xs font-medium text-gray-500">
                        Starts
                        <input v-model="sprint.start_date" type="date" required class="mt-1 block w-full rounded-lg text-sm" />
                    </label>
                    <label class="text-xs font-medium text-gray-500">
                        Ends
                        <input v-model="sprint.end_date" type="date" required class="mt-1 block w-full rounded-lg text-sm" />
                        <span v-if="sprint.errors.end_date" class="mt-1 block text-rose-600">{{ sprint.errors.end_date }}</span>
                    </label>
                    <button type="submit" :disabled="sprint.processing" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-indigo-50 shadow-sm hover:bg-indigo-500 disabled:opacity-50 sm:mt-5">Add</button>
                </form>
            </Panel>
        </div>
    </AuthenticatedLayout>
</template>
