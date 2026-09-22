<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import StatCard from '@/Components/Usage/StatCard.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { clock, count, duration, when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';
import { computed, ref } from 'vue';

const props = defineProps({
    session: Object,
    interactions: Array,
    canViewRaw: Boolean,
});

// Rows captured before the agent could tell a person's prompt from the agent's
// own work carry no kind. Guessing one from the old boolean produced confident
// nonsense — "you asked" on twelve agent steps — so where the answer is unknown
// this page says nothing rather than something wrong, and falls back to a plain
// list of exchanges in order.
const known = computed(() => props.interactions.every((i) => !i.legacy_kind));

const turns = computed(() => {
    const byTurn = new Map();

    for (const row of props.interactions) {
        if (!byTurn.has(row.turn)) {
            byTurn.set(row.turn, { turn: row.turn, asked: null, steps: [], answer: null });
        }
        const turn = byTurn.get(row.turn);

        if (row.kind === 'human' && !turn.asked) turn.asked = row;
        else if (row.final_answer) turn.answer = row;
        else turn.steps.push(row);
    }

    return [...byTurn.values()];
});

const humanTurns = computed(() => turns.value.filter((t) => t.asked).length);
const utilityCount = computed(() => props.interactions.filter((i) => i.kind === 'utility').length);
const agentSteps = computed(() => props.interactions.length - humanTurns.value - utilityCount.value);

// The exchanges worth reading: something was asked, something came back. The
// rest are the agent's mechanics and sit behind a toggle.
const withText = computed(() =>
    props.interactions.filter((i) => (i.answer_chars ?? 0) > 0 || (i.prompt_chars ?? 0) > 0),
);
const substantial = computed(() => props.interactions.filter((i) => (i.answer_chars ?? 0) > 40));

const showAll = ref(false);
const listed = computed(() => (showAll.value ? withText.value : substantial.value));

const open = ref(new Set());
const toggle = (turn) => {
    const next = new Set(open.value);
    next.has(turn) ? next.delete(turn) : next.add(turn);
    open.value = next;
};
</script>

<template>
    <Head :title="`Session ${session.id}`" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">
                    {{ session.project ?? 'Unknown project' }}
                </h2>
                <Tag v-if="session.branch" :label="session.branch" />
                <Tag v-if="session.tool" :label="session.tool" tone="blue" />
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-4xl space-y-6 px-4 sm:px-6 lg:px-8">
                <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
                    <template v-if="known">
                        <StatCard label="You asked" :value="count(humanTurns)" hint="turns you typed" />
                        <StatCard label="Agent steps" :value="count(agentSteps)" hint="work done on your behalf" />
                        <StatCard label="Tool's own calls" :value="count(utilityCount)" hint="grading, titles, suggestions" />
                    </template>
                    <template v-else>
                        <StatCard label="Exchanges" :value="count(interactions.length)" hint="requests in this session" />
                        <StatCard label="With a reply" :value="count(substantial.length)" hint="more than a token or two back" />
                        <StatCard
                            label="Tokens"
                            :value="count(interactions.reduce((n, i) => n + (i.prompt_tokens ?? 0) + (i.response_tokens ?? 0), 0))"
                        />
                    </template>
                    <StatCard label="AI time" :value="duration(session.seconds)" :hint="when(session.started_at)" />
                </div>

                <!-- ============ Sessions captured with kinds: turn by turn ============ -->
                <Panel v-if="known" title="The session, in order" :subtitle="session.repo ?? undefined">
                    <ol class="divide-y divide-gray-100">
                        <li v-for="t in turns" :key="t.turn" class="space-y-2 px-5 py-5">
                            <div v-if="t.asked">
                                <div class="flex items-center gap-2">
                                    <Tag label="you asked" tone="green" />
                                    <span class="text-xs tabular-nums text-gray-400">
                                        {{ clock(t.asked.occurred_at) }} · {{ count(t.asked.prompt_chars) }} chars
                                    </span>
                                    <Score :value="t.asked.score" class="ml-auto" />
                                    <Link :href="route('usage.show', t.asked.id)" class="text-xs text-gray-400 hover:text-gray-700">details</Link>
                                </div>
                                <p
                                    v-if="t.asked.prompt_preview"
                                    class="mt-1 whitespace-pre-wrap break-words rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-900"
                                >{{ t.asked.prompt_preview }}</p>
                            </div>
                            <div v-else class="text-xs text-gray-500">Work already under way when this session began.</div>

                            <div v-if="t.answer">
                                <div class="flex items-center gap-2">
                                    <Tag label="reply" tone="blue" />
                                    <span class="text-xs tabular-nums text-gray-400">
                                        {{ clock(t.answer.occurred_at) }} · {{ count(t.answer.answer_chars) }} chars
                                    </span>
                                    <Link :href="route('usage.show', t.answer.id)" class="ml-auto text-xs text-gray-400 hover:text-gray-700">details</Link>
                                </div>
                                <p
                                    v-if="t.answer.answer_preview"
                                    class="mt-1 whitespace-pre-wrap break-words rounded-lg border border-blue-100 bg-blue-50/60 px-3 py-2 text-sm text-gray-900"
                                >{{ t.answer.answer_preview }}</p>
                            </div>

                            <button
                                v-if="t.steps.length"
                                class="text-xs text-gray-500 hover:text-gray-900 hover:underline"
                                @click="toggle(t.turn)"
                            >
                                {{ open.has(t.turn) ? 'Hide' : 'Show' }} {{ t.steps.length }}
                                {{ t.steps.length === 1 ? 'step' : 'steps' }} in between
                            </button>

                            <ul v-if="open.has(t.turn)" class="space-y-1">
                                <li v-for="step in t.steps" :key="step.id" class="rounded-md bg-gray-50 px-3 py-2 text-xs">
                                    <div class="flex items-center gap-2">
                                        <span class="tabular-nums text-gray-400">{{ clock(step.occurred_at) }}</span>
                                        <Tag :label="step.kind" :tone="step.kind === 'utility' ? 'amber' : 'gray'" />
                                        <span class="text-gray-500">{{ step.model ?? 'unknown model' }}</span>
                                        <Link :href="route('usage.show', step.id)" class="ml-auto text-gray-400 hover:text-gray-700">details</Link>
                                    </div>
                                    <p v-if="step.answer_preview" class="mt-1 truncate text-gray-600">{{ step.answer_preview }}</p>
                                </li>
                            </ul>
                        </li>
                    </ol>
                </Panel>

                <!-- ====== Sessions captured before kinds: plain exchanges, no labels ====== -->
                <Panel v-else title="Exchanges" :subtitle="session.repo ?? undefined">
                    <template #actions>
                        <button class="text-gray-500 hover:text-gray-900" @click="showAll = !showAll">
                            {{ showAll ? 'Only ones with a real reply' : `Show all ${withText.length}` }}
                        </button>
                    </template>

                    <ul class="divide-y divide-gray-100">
                        <li v-for="i in listed" :key="i.id" class="px-5 py-4">
                            <div class="flex items-center gap-2 text-xs text-gray-400">
                                <span class="tabular-nums">{{ clock(i.occurred_at) }}</span>
                                <span>{{ i.model ?? 'unknown model' }}</span>
                                <span class="tabular-nums">
                                    {{ count(i.prompt_tokens) }}/{{ count(i.response_tokens) }} tokens
                                </span>
                                <Score :value="i.score" class="ml-auto" />
                                <Link :href="route('usage.show', i.id)" class="text-gray-400 hover:text-gray-700">details</Link>
                            </div>

                            <p
                                v-if="i.prompt_preview"
                                class="mt-2 whitespace-pre-wrap break-words rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-900"
                            >{{ i.prompt_preview }}</p>

                            <p
                                v-if="i.answer_preview"
                                class="mt-1 whitespace-pre-wrap break-words rounded-lg border border-blue-100 bg-blue-50/60 px-3 py-2 text-sm text-gray-900"
                            >{{ i.answer_preview }}</p>
                            <p v-else class="mt-1 text-xs italic text-gray-400">No reply text captured.</p>
                        </li>

                        <li v-if="!listed.length" class="px-5 py-10 text-center text-sm text-gray-500">
                            Nothing with text in this session.
                        </li>
                    </ul>
                </Panel>

                <p class="px-1 text-xs leading-relaxed text-gray-500">
                    <template v-if="known">
                        <strong class="text-gray-700">What you are reading.</strong> A turn is one message you typed,
                        the reply you got, and the agent's steps in between.
                    </template>
                    <template v-else>
                        <strong class="text-gray-700">Why there are no "you"/"agent" labels here.</strong>
                        This session was captured before the agent could tell your prompts from its own follow-ups,
                        so every exchange is listed in order and unlabelled. Sessions captured from now on are
                        grouped into turns.
                    </template>
                    Opening the full text of any exchange is recorded in the audit log.
                </p>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
