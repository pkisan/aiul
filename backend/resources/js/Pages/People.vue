<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { ago } from '@/Components/Usage/format';
import { Head, router, useForm, usePage } from '@inertiajs/vue3';
import { computed, ref } from 'vue';

defineProps({ people: Array, issued: Object });

const me = computed(() => usePage().props.auth.user);
const roles = [
    { value: 'member', label: 'Member', hint: 'sees only their own data' },
    { value: 'manager', label: 'Manager', hint: 'sees everyone’s usage' },
    { value: 'admin', label: 'Admin', hint: 'manages people' },
];

const form = useForm({ name: '', email: '', role: 'member', can_view_raw_prompts: false });
const add = () => form.post(route('people.store'), { preserveScroll: true, onSuccess: () => form.reset() });

// Role and prompt access save as soon as they change.
const save = (p, changes) =>
    router.patch(
        route('people.update', p.id),
        { role: p.role, can_view_raw_prompts: p.can_view_raw_prompts, ...changes },
        { preserveScroll: true },
    );

const reset = (p) => {
    if (confirm(`Give ${p.name} a new password? Their current one stops working and they are signed out everywhere.`)) {
        router.post(route('people.password', p.id), {}, { preserveScroll: true });
    }
};

const copied = ref(false);
const copy = (text) =>
    navigator.clipboard?.writeText(text).then(() => {
        copied.value = true;
        setTimeout(() => (copied.value = false), 1500);
    });
</script>

<template>
    <Head title="People" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <div>
                <h1 class="text-xl font-semibold text-gray-900">People</h1>
                <p class="mt-1 text-sm text-gray-500">
                    Everyone who can sign in. There is no public sign-up — add people here and send them their password.
                </p>
            </div>

            <div
                v-if="issued"
                class="rounded-xl border border-emerald-300 bg-emerald-50 px-5 py-4 dark:border-emerald-800 dark:bg-emerald-950/40"
                role="status"
            >
                <p class="text-sm font-medium text-emerald-900 dark:text-emerald-200">
                    Password for {{ issued.email }} — shown once, copy it now:
                </p>
                <div class="mt-2 flex items-center gap-3">
                    <code class="rounded-md bg-white px-3 py-1.5 font-mono text-sm text-gray-900 ring-1 ring-emerald-200 dark:ring-emerald-800">{{ issued.password }}</code>
                    <button type="button" class="text-sm font-medium text-emerald-800 hover:underline dark:text-emerald-300" @click="copy(issued.password)">
                        {{ copied ? 'Copied' : 'Copy' }}
                    </button>
                </div>
            </div>

            <Panel title="Add a person">
                <form class="grid gap-4 px-5 py-4 sm:grid-cols-[1fr_1fr_10rem_auto] sm:items-start" @submit.prevent="add">
                    <label class="text-xs font-medium text-gray-500">
                        Name
                        <input v-model="form.name" type="text" required maxlength="255" class="mt-1 block w-full rounded-lg text-sm" />
                        <span v-if="form.errors.name" class="mt-1 block text-rose-600">{{ form.errors.name }}</span>
                    </label>
                    <label class="text-xs font-medium text-gray-500">
                        Email
                        <input v-model="form.email" type="email" required maxlength="255" class="mt-1 block w-full rounded-lg text-sm" />
                        <span v-if="form.errors.email" class="mt-1 block text-rose-600">{{ form.errors.email }}</span>
                    </label>
                    <label class="text-xs font-medium text-gray-500">
                        Role
                        <select v-model="form.role" class="mt-1 block w-full rounded-lg text-sm">
                            <option v-for="r in roles" :key="r.value" :value="r.value">{{ r.label }}</option>
                        </select>
                    </label>
                    <button
                        type="submit"
                        :disabled="form.processing"
                        class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-indigo-50 shadow-sm hover:bg-indigo-500 disabled:opacity-50 sm:mt-5"
                    >Add</button>
                    <label v-if="form.role === 'admin'" class="flex items-center gap-2 text-sm text-gray-600 sm:col-span-4">
                        <input v-model="form.can_view_raw_prompts" type="checkbox" class="rounded text-indigo-600" />
                        May read prompt text
                    </label>
                </form>
            </Panel>

            <Panel :title="`${people.length} ${people.length === 1 ? 'person' : 'people'}`">
                <div class="overflow-x-auto">
                    <table class="min-w-full text-sm">
                        <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                            <tr>
                                <th class="px-5 py-2.5">Person</th>
                                <th class="px-3 py-2.5">Role</th>
                                <th class="px-3 py-2.5">Prompt text</th>
                                <th class="px-3 py-2.5 text-right">Devices</th>
                                <th class="px-3 py-2.5 text-right">Last AI use</th>
                                <th class="px-5 py-2.5"><span class="sr-only">Actions</span></th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100">
                            <tr v-for="p in people" :key="p.id">
                                <td class="px-5 py-3">
                                    <div class="font-medium text-gray-900">
                                        {{ p.name }}
                                        <span v-if="p.id === me.id" class="ml-1 text-xs font-normal text-gray-400">(you)</span>
                                    </div>
                                    <div class="text-xs text-gray-500">{{ p.email }}</div>
                                </td>
                                <td class="px-3 py-3">
                                    <select
                                        :value="p.role"
                                        :disabled="p.id === me.id"
                                        :aria-label="`Role of ${p.name}`"
                                        class="rounded-lg py-1 pl-2 pr-8 text-sm disabled:opacity-60"
                                        @change="save(p, { role: $event.target.value })"
                                    >
                                        <option v-for="r in roles" :key="r.value" :value="r.value">{{ r.label }}</option>
                                    </select>
                                </td>
                                <td class="px-3 py-3">
                                    <label v-if="p.role === 'admin'" class="inline-flex items-center gap-2 text-gray-600">
                                        <input
                                            type="checkbox"
                                            class="rounded text-indigo-600"
                                            :checked="p.can_view_raw_prompts"
                                            @change="save(p, { can_view_raw_prompts: $event.target.checked })"
                                        />
                                        Can read
                                    </label>
                                    <span v-else class="text-gray-400">—</span>
                                </td>
                                <td class="px-3 py-3 text-right tabular-nums text-gray-600">{{ p.devices }}</td>
                                <td class="px-3 py-3 text-right text-gray-600">{{ p.last_active ? ago(p.last_active) : 'never' }}</td>
                                <td class="px-5 py-3 text-right">
                                    <button
                                        v-if="p.id !== me.id"
                                        type="button"
                                        class="whitespace-nowrap text-sm text-gray-500 hover:text-gray-900 hover:underline"
                                        @click="reset(p)"
                                    >Reset password</button>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </Panel>
        </div>
    </AuthenticatedLayout>
</template>
