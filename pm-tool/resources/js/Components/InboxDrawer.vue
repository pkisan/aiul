<script setup>
// The unlinked inbox: AI sessions the linker could not place for sure. Opens
// from the "Inbox" button in the top bar; loads its own data (JSON), so it
// works on every page without each page passing it.
import { clock, toolName } from '@/Components/Usage/format';
import { router, usePage } from '@inertiajs/vue3';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { statusLabels } from '../pm.js';

const props = defineProps({ open: Boolean });
const emit = defineEmits(['close']);

const isManager = computed(() => ['manager', 'admin'].includes(usePage().props.auth.user?.role));
const scope = ref('mine');
const data = ref(null);
const error = ref(null);
const busy = ref(null); // id of the session being saved

const why = {
    explicit: 'you were working on it',
    convention: 'its key is in the branch name',
    time_window: 'the only task in progress then',
};

const load = async () => {
    error.value = null;
    try {
        data.value = (await window.axios.get(route('pm.inbox.index'), { params: { scope: scope.value } })).data;
    } catch {
        error.value = 'Could not load the inbox. Try again.';
    }
};

const decide = async (item, action, taskId = null) => {
    if (action === 'assign' && !taskId) return;
    busy.value = item.id;
    try {
        await window.axios.post(route('pm.inbox.decide', item.id), { action, task_id: taskId });
        data.value.items = data.value.items.filter((i) => i.id !== item.id);
        data.value.total--;
        router.reload({ only: ['pmInboxCount'] });
    } catch {
        error.value = 'That did not save. Try again.';
    } finally {
        busy.value = null;
    }
};

watch(() => props.open, (open) => open && load());
watch(scope, load);

const onKey = (e) => e.key === 'Escape' && props.open && emit('close');
onMounted(() => document.addEventListener('keydown', onKey));
onBeforeUnmount(() => document.removeEventListener('keydown', onKey));

const day = (v) => new Date(v).toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
// My tasks first in the picker, then the rest.
const choices = computed(() => {
    const tasks = data.value?.tasks ?? [];
    return [...tasks.filter((t) => t.mine), ...tasks.filter((t) => !t.mine)];
});
</script>

<template>
    <div v-if="open" class="fixed inset-0 z-40">
        <div class="absolute inset-0 bg-gray-900/40" aria-hidden="true" @click="emit('close')"></div>

        <aside
            class="absolute inset-y-0 right-0 flex w-full max-w-md flex-col bg-white shadow-xl"
            role="dialog"
            aria-modal="true"
            aria-labelledby="inbox-title"
        >
            <header class="border-b border-gray-200 px-5 py-4">
                <div class="flex items-center justify-between gap-3">
                    <h2 id="inbox-title" class="text-base font-semibold text-gray-900">Inbox</h2>
                    <button type="button" class="rounded-lg p-1 text-gray-500 hover:bg-gray-100" aria-label="Close" @click="emit('close')">
                        <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 18L18 6M6 6l12 12" /></svg>
                    </button>
                </div>
                <p class="mt-1 text-sm text-gray-500">AI sessions we could not link to a task for sure. Confirm, pick the task, or mark them not task work.</p>
                <div v-if="isManager" class="mt-3 inline-flex rounded-lg bg-gray-100 p-0.5 text-sm">
                    <button
                        v-for="s in [['mine', 'Mine'], ['team', 'Everyone']]"
                        :key="s[0]"
                        type="button"
                        class="rounded-md px-3 py-1"
                        :class="scope === s[0] ? 'bg-white font-medium text-gray-900 shadow-sm' : 'text-gray-500'"
                        :aria-pressed="scope === s[0]"
                        @click="scope = s[0]"
                    >{{ s[1] }}</button>
                </div>
            </header>

            <div class="flex-1 overflow-y-auto">
                <p v-if="error" class="m-5 rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300" role="alert">{{ error }}</p>
                <p v-if="!data && !error" class="px-5 py-10 text-center text-sm text-gray-500">Loading…</p>
                <p v-else-if="data && !data.items.length" class="px-5 py-10 text-center text-sm text-gray-500">Nothing waiting. Every AI session in the last 30 days is linked or sorted.</p>

                <ul v-if="data" class="divide-y divide-gray-100">
                    <li v-for="item in data.items" :key="item.id" class="space-y-2 px-5 py-4" :class="busy === item.id ? 'opacity-50' : ''">
                        <div class="flex flex-wrap items-baseline justify-between gap-x-3 text-xs text-gray-500">
                            <span class="font-medium text-gray-900">{{ toolName(item.tool) }}<template v-if="data.team"> · {{ item.person }}</template></span>
                            <span>{{ day(item.started_at) }}, {{ clock(item.started_at) }}–{{ clock(item.ended_at) }} · {{ item.prompts }} {{ item.prompts === 1 ? 'prompt' : 'prompts' }}</span>
                        </div>
                        <p v-if="item.where" class="break-all font-mono text-xs text-gray-500">{{ item.where }}<template v-if="item.branch"> · {{ item.branch }}</template></p>
                        <p v-if="item.preview" class="text-sm text-gray-700">“{{ item.preview }}”</p>

                        <div v-if="item.suggestion" class="rounded-lg bg-indigo-50 px-3 py-2 text-sm dark:bg-indigo-950/50">
                            <p class="text-indigo-900 dark:text-indigo-200">
                                Looks like <span class="font-medium">{{ item.suggestion.key }} · {{ item.suggestion.title }}</span>{{ ' ' }}<span class="text-indigo-700 dark:text-indigo-300">({{ why[item.suggestion.method] }})</span>
                            </p>
                            <button
                                type="button"
                                :disabled="busy === item.id"
                                class="mt-2 rounded-lg bg-indigo-600 px-3 py-1 text-xs font-medium text-indigo-50 hover:bg-indigo-500"
                                @click="decide(item, 'confirm')"
                            >Yes, {{ item.suggestion.key }}</button>
                        </div>

                        <div class="flex flex-wrap items-center gap-2">
                            <select
                                :disabled="busy === item.id"
                                :aria-label="`Pick the task for this ${toolName(item.tool)} session`"
                                class="min-w-0 flex-1 rounded-lg py-1 text-sm"
                                @change="decide(item, 'assign', $event.target.value)"
                            >
                                <option value="">{{ item.suggestion ? 'Another task…' : 'Pick the task…' }}</option>
                                <option v-for="t in choices" :key="t.id" :value="t.id">{{ t.key }} · {{ t.title }} ({{ statusLabels[t.status] }})</option>
                            </select>
                            <button
                                type="button"
                                :disabled="busy === item.id"
                                class="rounded-lg px-3 py-1 text-sm text-gray-600 ring-1 ring-gray-200 hover:bg-gray-50"
                                @click="decide(item, 'none')"
                            >Not task work</button>
                        </div>
                    </li>
                </ul>
                <p v-if="data && data.total > data.items.length" class="px-5 py-4 text-center text-xs text-gray-500">
                    Showing the latest {{ data.items.length }} of {{ data.total }}.
                </p>
            </div>
        </aside>
    </div>
</template>
