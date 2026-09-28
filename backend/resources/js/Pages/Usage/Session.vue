<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Markdown from '@/Components/Usage/Markdown.vue';
import Message from '@/Components/Usage/Message.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { ago, clock, count, duration, toolName, when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';
import { computed, reactive, ref } from 'vue';

const props = defineProps({
    session: Object,
    interactions: Array,
    canViewRaw: Boolean,
    sidebar: { type: Array, default: () => [] },
});

// The session read the way the person saw it in their AI tool:
//
//   what they typed  — a grey bubble on the right, verbatim
//   agent steps      — one folded line above the answer: the agent feeding
//                      itself tool results is NOT the person talking
//   the answer       — no bubble, rendered markdown, full column width
//
// The tool's own housekeeping calls (titles, grading) belong to nobody's turn
// and stay hidden unless asked for.
const showTool = ref(false);
const openSteps = reactive({});
const openPrompt = reactive({});
const PROMPT_FOLD = 1200;

const turns = computed(() => {
    const out = [];
    let current = null;

    for (const i of props.interactions) {
        if (i.kind === 'utility' && !showTool.value) continue;

        if (i.kind === 'human' || !current) {
            current = { key: i.id, asked: i.kind === 'human' ? i : null, steps: [], answer: null };
            out.push(current);
            if (i.kind === 'human') {
                // A plain chat (claude.ai, ChatGPT) answers in the same exchange.
                if (i.final_answer) current.answer = i;
                continue;
            }
        }

        if (i.final_answer) current.answer = i;
        else current.steps.push(i);
    }

    return out;
});

const humanTurns = computed(() => props.interactions.filter((i) => i.kind === 'human').length);
const agentSteps = computed(() => props.interactions.filter((i) => i.kind === 'agent').length);
const utilityCount = computed(() => props.interactions.filter((i) => i.kind === 'utility').length);
const tokens = (rows) => rows.reduce((n, i) => n + (i.prompt_tokens ?? 0) + (i.response_tokens ?? 0), 0);

const title = (s) => s.project ?? toolName(s.tool);
const who = computed(() => props.session.accounts[0] ?? props.session.person ?? 'Person');
const folded = (t) => t.asked.prompt_preview.length > PROMPT_FOLD && !openPrompt[t.key];
</script>

<template>
    <Head :title="title(session)" />

    <AuthenticatedLayout>
        <!-- Full width under the app bar (4rem): sidebar and conversation scroll
             on their own, so the list stays put while a long session is read. -->
        <div class="flex h-[calc(100vh-4rem)] bg-white">
            <aside class="hidden w-72 shrink-0 flex-col border-r border-gray-200 bg-gray-50 md:flex">
                <div class="px-4 pb-2 pt-4">
                    <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                    <p class="mt-4 text-xs font-medium uppercase tracking-wide text-gray-400">
                        Sessions<template v-if="session.person"> · {{ session.person }}</template>
                    </p>
                </div>
                <nav class="flex-1 space-y-0.5 overflow-y-auto px-2 pb-4">
                    <Link
                        v-for="s in sidebar"
                        :key="s.id"
                        :href="route('usage.session', s.id)"
                        class="block rounded-lg px-3 py-2 hover:bg-gray-200/60"
                        :class="s.id === session.id ? 'bg-gray-200/80' : ''"
                        :aria-current="s.id === session.id ? 'page' : undefined"
                    >
                        <span class="block truncate text-sm text-gray-900">{{ title(s) }}</span>
                        <span class="mt-0.5 block truncate text-xs text-gray-500">
                            {{ toolName(s.tool) }} · {{ ago(s.ended_at ?? s.started_at) }} ·
                            {{ s.human_prompts }} prompt{{ s.human_prompts === 1 ? '' : 's' }}
                        </span>
                    </Link>
                    <p v-if="!sidebar.length" class="px-3 py-2 text-sm text-gray-500">No other sessions in 30 days.</p>
                </nav>
            </aside>

            <main class="flex-1 overflow-y-auto">
                <header class="sticky top-0 z-10 border-b border-gray-100 bg-white/90 px-6 py-3 backdrop-blur">
                    <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                        <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900 md:hidden">←</Link>
                        <h1 class="truncate text-base font-semibold text-gray-900">{{ title(session) }}</h1>
                        <Tag v-if="session.tool" :label="toolName(session.tool)" tone="blue" />
                        <Tag v-if="session.branch" :label="session.branch" />
                        <span class="text-sm text-gray-500">
                            <template v-if="session.accounts.length">as {{ session.accounts.join(', ') }}</template>
                            <template v-if="session.person">
                                {{ session.accounts.length ? '·' : '' }} device of {{ session.person }}</template>
                        </span>
                    </div>
                    <div class="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500">
                        <span :title="when(session.started_at)">{{ when(session.started_at) }}</span>
                        <span>{{ count(humanTurns) }} typed</span>
                        <span>{{ count(agentSteps) }} agent steps</span>
                        <span>{{ count(tokens(interactions)) }} tokens</span>
                        <span>{{ duration(session.seconds) }} AI time</span>
                        <span v-if="session.repo" class="truncate font-mono">{{ session.repo }}</span>
                        <button v-if="utilityCount" class="ml-auto text-gray-500 underline-offset-2 hover:text-gray-900 hover:underline" @click="showTool = !showTool">
                            {{ showTool ? "Hide the tool's own calls" : `Show the tool's own calls (${utilityCount})` }}
                        </button>
                    </div>
                </header>

                <p v-if="!canViewRaw" class="px-6 py-16 text-center text-sm text-gray-500">
                    You do not have permission to read prompt text.
                </p>

                <div v-else class="mx-auto max-w-3xl space-y-12 px-6 py-10">
                    <section v-for="t in turns" :key="t.key" class="group space-y-5">
                        <!-- What the person typed, verbatim -->
                        <div v-if="t.asked" class="flex flex-col items-end">
                            <div class="max-w-[85%] rounded-2xl bg-gray-100 px-4 py-3 text-[15px] leading-relaxed text-gray-900">
                                <p v-if="t.asked.prompt_preview" class="whitespace-pre-wrap break-words">{{
                                    folded(t) ? t.asked.prompt_preview.slice(0, PROMPT_FOLD) + '…' : t.asked.prompt_preview
                                }}</p>
                                <p v-else class="italic text-gray-500">(only context the tool added — nothing typed)</p>
                                <button
                                    v-if="(t.asked.prompt_preview?.length ?? 0) > PROMPT_FOLD"
                                    class="mt-2 text-sm text-gray-600 hover:text-gray-900"
                                    @click="openPrompt[t.key] = !openPrompt[t.key]"
                                >{{ openPrompt[t.key] ? 'Show less' : 'Show more' }}</button>
                            </div>
                            <div class="mt-1.5 flex items-center gap-2 text-xs text-gray-400">
                                <span>{{ t.asked.account ?? who }}</span>
                                <span class="tabular-nums">{{ clock(t.asked.occurred_at) }}</span>
                                <span title="Prompt quality score"><Score :value="t.asked.score" /></span>
                                <Link :href="route('usage.show', t.asked.id)" class="hover:text-gray-700">details</Link>
                            </div>
                        </div>

                        <!-- The agent working on its own, one folded line -->
                        <div v-if="t.steps.length">
                            <button
                                class="inline-flex items-center gap-1.5 text-sm text-gray-500 hover:text-gray-900"
                                @click="openSteps[t.key] = !openSteps[t.key]"
                            >
                                <span>Agent worked on its own: {{ t.steps.length }} step{{ t.steps.length === 1 ? '' : 's' }}
                                    · {{ count(tokens(t.steps)) }} tokens</span>
                                <span class="transition-transform" :class="openSteps[t.key] ? 'rotate-90' : ''">›</span>
                            </button>
                            <div v-if="openSteps[t.key]" class="mt-3 space-y-2 border-l-2 border-gray-100 pl-4">
                                <Message
                                    v-for="s in t.steps"
                                    :key="s.id"
                                    :who="s.kind === 'utility' ? 'tool' : 'agent'"
                                    :text="s.prompt_preview || s.answer_preview"
                                    placeholder="(no text)"
                                    :fold="300"
                                    class="!ml-0 !max-w-full"
                                >
                                    <template #meta>
                                        <span class="font-medium">{{ s.kind === 'utility' ? "tool's own call" : 'agent step' }}</span>
                                        <span class="tabular-nums">{{ clock(s.occurred_at) }}</span>
                                        <span>{{ s.model }}</span>
                                        <Link :href="route('usage.show', s.id)" class="underline">open</Link>
                                    </template>
                                </Message>
                            </div>
                        </div>

                        <!-- The answer the person read, as they read it -->
                        <div v-if="t.answer">
                            <Markdown v-if="t.answer.answer_preview" :text="t.answer.answer_preview" />
                            <p v-else class="text-sm italic text-gray-400">(no answer text captured)</p>
                            <div class="mt-3 flex items-center gap-2 text-xs text-gray-400">
                                <span class="font-medium text-gray-500">{{ t.answer.model ?? 'AI' }}</span>
                                <span class="tabular-nums">{{ clock(t.answer.occurred_at) }}</span>
                                <Link :href="route('usage.show', t.answer.id)" class="hover:text-gray-700">details</Link>
                            </div>
                        </div>
                        <p v-else-if="t.asked" class="text-sm italic text-gray-400">No answer text captured for this turn.</p>
                    </section>

                    <p v-if="!turns.length" class="py-6 text-center text-sm text-gray-500">Nothing in this session.</p>

                    <p class="border-t border-gray-100 pt-6 text-xs leading-relaxed text-gray-400">
                        Grey bubbles on the right are what the person typed. Text without a bubble is the answer they
                        read. "Agent worked on its own" lines are the agent feeding itself tool results — not the
                        person. Opening this page is recorded in the audit log.
                    </p>
                </div>
            </main>
        </div>
    </AuthenticatedLayout>
</template>
