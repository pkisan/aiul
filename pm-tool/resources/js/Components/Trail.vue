<script setup>
// The Task AI Trail: every AI session behind a task, top to bottom, each
// prompt numbered session.prompt (1.1, 1.2 ...) and folded to one line.
// Opening a row loads that prompt and its answer.
import Message from '@/Components/Usage/Message.vue';
import { clock, count, duration, toolName } from '@/Components/Usage/format';
import { computed, reactive } from 'vue';
import { usd } from '../pm.js';

const props = defineProps({ trail: Object, taskKey: String });

const t = computed(() => props.trail.totals);
const sessions = computed(() => props.trail.sessions);
const day = (v) => new Date(v).toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
const sameDay = computed(() => t.value.from && new Date(t.value.from).toDateString() === new Date(t.value.to).toDateString());
const span = computed(() =>
    !t.value.from ? null : sameDay.value ? `${day(t.value.from)}, ${clock(t.value.from)} → ${clock(t.value.to)}` : `${day(t.value.from)} → ${day(t.value.to)}`,
);
const tools = computed(() => [...new Set(sessions.value.map((s) => toolName(s.tool)))].join(', '));

const how = {
    explicit: 'Started working',
    convention: 'Key in branch',
    time_window: 'Only task in progress',
    manual: 'Picked in inbox',
};

// Opened rows and their text, loaded once.
const open = reactive({});
const toggle = async (turn) => {
    if (open[turn.id]) {
        delete open[turn.id];
        return;
    }
    open[turn.id] = { loading: true };
    try {
        open[turn.id] = (await window.axios.get(route('pm.trail.turn', turn.id))).data;
    } catch {
        open[turn.id] = { error: true };
    }
};
</script>

<template>
    <section class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
        <header class="border-b border-gray-100 px-5 py-4">
            <h3 class="text-sm font-semibold text-gray-900">AI trail</h3>
            <p v-if="sessions.length" class="mt-0.5 text-sm text-gray-700">
                {{ t.prompts }} {{ t.prompts === 1 ? 'prompt' : 'prompts' }} in {{ sessions.length }} {{ sessions.length === 1 ? 'session' : 'sessions' }} with {{ tools }}<template v-if="span">, {{ span }}</template>.
            </p>
            <dl v-if="sessions.length" class="mt-3 grid grid-cols-2 gap-3 text-xs sm:grid-cols-4">
                <div><dt class="text-gray-500">Prompts</dt><dd class="text-base font-semibold tabular-nums text-gray-900">{{ t.prompts }}</dd></div>
                <div><dt class="text-gray-500">Tokens</dt><dd class="text-base font-semibold tabular-nums text-gray-900">{{ count(t.cost.tokens) }}</dd></div>
                <div><dt class="text-gray-500">Cost (estimate)</dt><dd class="text-base font-semibold tabular-nums text-gray-900">{{ usd(t.cost.usd) }}</dd></div>
                <div><dt class="text-gray-500">AI time</dt><dd class="text-base font-semibold tabular-nums text-gray-900">{{ duration(t.seconds) }}</dd></div>
            </dl>
            <p v-if="t.suggested" class="mt-3 text-xs text-amber-800 dark:text-amber-300">
                {{ t.suggested }} of these {{ t.suggested === 1 ? 'session is a suggestion' : 'sessions are suggestions' }} nobody has confirmed yet (marked below). Confirm them in the Inbox.
            </p>
        </header>

        <p v-if="!sessions.length" class="px-5 py-6 text-sm text-gray-500">
            No AI sessions linked yet. They link here when someone presses Start working, works on a branch with {{ taskKey }} in its name, or has this as their only task in progress.
        </p>

        <ol v-else class="relative px-5 py-4">
            <li v-for="s in sessions" :key="s.id" class="relative pb-5 pl-6 last:pb-0">
                <!-- the timeline rail and this session's dot -->
                <span class="absolute bottom-0 left-[5px] top-2 w-px bg-gray-200" aria-hidden="true"></span>
                <span class="absolute left-0 top-1.5 h-[11px] w-[11px] rounded-full border-2 border-white bg-indigo-600 dark:bg-indigo-400" aria-hidden="true"></span>

                <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-xs">
                    <span class="font-semibold text-gray-900">Session {{ s.n }}</span>
                    <span class="text-gray-700">{{ toolName(s.tool) }}<template v-if="s.person"> · {{ s.person }}</template></span>
                    <span class="text-gray-500">{{ day(s.started_at) }}, {{ clock(s.started_at) }} → {{ clock(s.ended_at) }}</span>
                    <span class="rounded-full bg-gray-100 px-2 py-0.5 text-gray-600">{{ how[s.method] ?? s.method }}</span>
                    <span v-if="!s.confirmed && s.confidence < 0.8" class="rounded-full bg-amber-100 px-2 py-0.5 font-medium text-amber-900 dark:bg-amber-900/40 dark:text-amber-200">suggested</span>
                </div>
                <p v-if="s.models.length" class="mt-0.5 font-mono text-[11px] text-gray-500">{{ s.models.join(', ') }}</p>

                <ul class="mt-2 space-y-1">
                    <li v-for="turn in s.turns" :key="turn.id">
                        <button
                            type="button"
                            class="flex w-full items-baseline gap-3 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-gray-50"
                            :aria-expanded="!!open[turn.id]"
                            :disabled="!s.canRead"
                            @click="toggle(turn)"
                        >
                            <span class="w-8 shrink-0 font-mono text-xs text-gray-500">{{ turn.n }}</span>
                            <span class="w-16 shrink-0 text-xs tabular-nums text-gray-500">{{ clock(turn.at) }}</span>
                            <span class="min-w-0 flex-1 truncate" :class="turn.preview ? 'text-gray-900' : 'italic text-gray-500'">
                                {{ turn.preview ?? (s.canRead ? 'Prompt' : `Prompt text is visible to ${s.person ?? 'its author'} and managers`) }}
                            </span>
                            <span class="hidden shrink-0 text-xs tabular-nums text-gray-500 sm:inline">{{ count(turn.tokens) }} tokens<template v-if="turn.steps"> · +{{ turn.steps }} agent {{ turn.steps === 1 ? 'step' : 'steps' }}</template></span>
                        </button>
                        <div v-if="open[turn.id]" class="ml-2 space-y-3 border-l-2 border-indigo-200 py-2 pl-4 dark:border-indigo-800">
                            <p v-if="open[turn.id].loading" class="text-xs text-gray-500">Loading…</p>
                            <p v-else-if="open[turn.id].error" class="text-xs text-rose-600">Could not load this prompt.</p>
                            <template v-else>
                                <Message who="you" :text="open[turn.id].prompt" placeholder="No prompt text stored." />
                                <Message who="assistant" :text="open[turn.id].answer" placeholder="No answer text stored." />
                            </template>
                        </div>
                    </li>
                    <li v-if="!s.turns.length" class="px-2 text-xs text-gray-500">{{ s.steps }} automated agent steps, no typed prompt.</li>
                </ul>
            </li>
        </ol>
    </section>
</template>
