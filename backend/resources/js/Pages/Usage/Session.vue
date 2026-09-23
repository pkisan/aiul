<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Message from '@/Components/Usage/Message.vue';
import Panel from '@/Components/Usage/Panel.vue';
import StatCard from '@/Components/Usage/StatCard.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { clock, count, duration, when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';
import { computed, reactive, ref } from 'vue';

const props = defineProps({
    session: Object,
    interactions: Array,
    canViewRaw: Boolean,
});

// The session read as a chat. Each turn is one thing the person typed, the steps
// the agent took on its own to do it, and the answer they read:
//
//   you asked      — right, dark bubble
//   agent steps    — folded between, dashed and grey: the agent feeding itself
//                    tool results is NOT the person talking
//   answer         — left, white bubble
//
// The tool's own housekeeping calls (titles, grading) belong to nobody's turn
// and stay hidden unless asked for.
const showTool = ref(false);
const openSteps = reactive({});

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
</script>

<template>
    <Head :title="`Session ${session.id}`" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex flex-wrap items-center gap-3">
                <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">
                    {{ session.project ?? 'Unknown project' }}
                </h2>
                <Tag v-if="session.branch" :label="session.branch" />
                <Tag v-if="session.tool" :label="session.tool" tone="blue" />
                <span class="text-sm text-gray-500">
                    <template v-if="session.accounts.length">as {{ session.accounts.join(', ') }}</template>
                    <template v-if="session.person">
                        {{ session.accounts.length ? '·' : '' }} device of {{ session.person }}</template>
                </span>
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-4xl space-y-6 px-4 sm:px-6 lg:px-8">
                <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
                    <StatCard label="You asked" :value="count(humanTurns)" hint="messages typed by the person" />
                    <StatCard label="Agent steps" :value="count(agentSteps)" hint="the agent working on its own" />
                    <StatCard label="Tokens" :value="count(tokens(interactions))" />
                    <StatCard label="AI time" :value="duration(session.seconds)" :hint="when(session.started_at)" />
                </div>

                <Panel title="Conversation" :subtitle="session.repo ?? undefined">
                    <template #actions>
                        <button v-if="utilityCount" class="text-gray-500 hover:text-gray-900" @click="showTool = !showTool">
                            {{ showTool ? "Hide the tool's own calls" : `Show the tool's own calls (${utilityCount})` }}
                        </button>
                    </template>

                    <p v-if="!canViewRaw" class="px-5 py-10 text-center text-sm text-gray-500">
                        You do not have permission to read prompt text.
                    </p>

                    <div v-else class="space-y-8 bg-gray-50/60 px-5 py-6">
                        <section v-for="t in turns" :key="t.key" class="space-y-3">
                            <!-- What the person typed -->
                            <template v-if="t.asked">
                                <div class="flex items-center justify-end gap-2 text-[11px] text-gray-400">
                                    <span class="font-medium text-gray-600">{{ t.asked.account ?? session.person ?? 'Person' }}</span>
                                    <span class="tabular-nums">{{ clock(t.asked.occurred_at) }}</span>
                                    <Score :value="t.asked.score" />
                                    <Link :href="route('usage.show', t.asked.id)" class="hover:text-gray-700">details</Link>
                                </div>
                                <Message who="you" :text="t.asked.prompt_preview" placeholder="(only context the tool added — nothing typed)" />
                            </template>

                            <!-- The agent working on its own, folded -->
                            <div v-if="t.steps.length" class="ml-6">
                                <button
                                    class="inline-flex items-center gap-2 rounded-full border border-dashed border-gray-300 bg-white px-3 py-1 text-xs text-gray-600 hover:bg-gray-100"
                                    @click="openSteps[t.key] = !openSteps[t.key]"
                                >
                                    <span>{{ openSteps[t.key] ? '▾' : '▸' }}</span>
                                    Agent worked on its own: {{ t.steps.length }} step{{ t.steps.length === 1 ? '' : 's' }}
                                    · {{ count(tokens(t.steps)) }} tokens
                                </button>

                                <div v-if="openSteps[t.key]" class="mt-3 space-y-2">
                                    <div v-for="s in t.steps" :key="s.id" class="space-y-1">
                                        <Message :who="s.kind === 'utility' ? 'tool' : 'agent'" :text="s.prompt_preview || s.answer_preview" placeholder="(no text)" :fold="300">
                                            <template #meta>
                                                <span class="font-medium">{{ s.kind === 'utility' ? "tool's own call" : 'agent step' }}</span>
                                                <span class="tabular-nums">{{ clock(s.occurred_at) }}</span>
                                                <span>{{ s.model }}</span>
                                                <Link :href="route('usage.show', s.id)" class="underline">open</Link>
                                            </template>
                                        </Message>
                                    </div>
                                </div>
                            </div>

                            <!-- The answer the person read -->
                            <template v-if="t.answer">
                                <div class="flex items-center gap-2 text-[11px] text-gray-400">
                                    <span class="font-medium uppercase tracking-wide text-gray-500">{{ t.answer.model ?? 'AI' }}</span>
                                    <span class="tabular-nums">{{ clock(t.answer.occurred_at) }}</span>
                                    <Link :href="route('usage.show', t.answer.id)" class="hover:text-gray-700">details</Link>
                                </div>
                                <Message who="assistant" :text="t.answer.answer_preview" placeholder="(no answer text captured)" />
                            </template>
                            <p v-else-if="t.asked" class="text-xs italic text-gray-400">No answer text captured for this turn.</p>
                        </section>

                        <p v-if="!turns.length" class="py-6 text-center text-sm text-gray-500">Nothing in this session.</p>
                    </div>
                </Panel>

                <p class="px-1 text-xs leading-relaxed text-gray-500">
                    <strong class="text-gray-700">Reading this page.</strong> Dark bubbles on the right are what the
                    person typed. White bubbles on the left are the answer they read. Dashed grey steps are the agent
                    feeding itself tool results while it works — not the person. Opening this page is recorded in the
                    audit log.
                </p>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
