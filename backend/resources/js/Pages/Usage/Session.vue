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

// One thing the person asked for, and everything the agent did about it.
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

// Rows captured before kinds existed fall back to the old boolean, which was
// backwards. Say so on the page rather than letting it read as fact.
const legacy = computed(() => props.interactions.some((i) => i.legacy_kind));

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
                    <StatCard label="You asked" :value="count(humanTurns)" hint="turns you typed" />
                    <StatCard label="Agent steps" :value="count(agentSteps)" hint="work done on your behalf" />
                    <StatCard label="Tool's own calls" :value="count(utilityCount)" hint="grading, titles, suggestions" />
                    <StatCard label="AI time" :value="duration(session.seconds)" :hint="when(session.started_at)" />
                </div>

                <div
                    v-if="legacy"
                    class="rounded-xl border border-amber-200 bg-amber-50 px-5 py-3 text-sm text-amber-900"
                >
                    Captured before this agent could tell a person's prompt from the agent's own work. The
                    turns below are a guess from the old flag, which was often wrong — newer sessions are
                    grouped correctly.
                </div>

                <Panel title="The session, in order" :subtitle="session.repo ?? undefined">
                    <ol class="divide-y divide-gray-100">
                        <li v-for="t in turns" :key="t.turn" class="space-y-2 px-5 py-5">
                            <!-- What the person typed, in their own words. -->
                            <div v-if="t.asked">
                                <div class="flex items-center gap-2">
                                    <Tag label="you asked" tone="green" />
                                    <span class="text-xs tabular-nums text-gray-400">
                                        {{ clock(t.asked.occurred_at) }} · {{ count(t.asked.prompt_chars) }} chars
                                    </span>
                                    <Score :value="t.asked.score" class="ml-auto" />
                                    <Link
                                        :href="route('usage.show', t.asked.id)"
                                        class="text-xs text-gray-400 hover:text-gray-700"
                                    >details</Link>
                                </div>
                                <p
                                    v-if="t.asked.prompt_preview"
                                    class="mt-1 whitespace-pre-wrap break-words rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-900"
                                >{{ t.asked.prompt_preview }}</p>
                                <p v-else class="mt-1 text-sm italic text-gray-400">
                                    {{ canViewRaw ? 'No prompt text was captured.' : 'You cannot read prompt text.' }}
                                </p>
                            </div>
                            <div v-else class="text-xs text-gray-500">
                                Work already under way when this session began.
                            </div>

                            <!-- The reply, which is what the person actually read. -->
                            <div v-if="t.answer">
                                <div class="flex items-center gap-2">
                                    <Tag label="reply" tone="blue" />
                                    <span class="text-xs tabular-nums text-gray-400">
                                        {{ clock(t.answer.occurred_at) }} · {{ count(t.answer.answer_chars) }} chars ·
                                        {{ count(t.answer.response_tokens) }} tokens
                                    </span>
                                    <Link
                                        :href="route('usage.show', t.answer.id)"
                                        class="ml-auto text-xs text-gray-400 hover:text-gray-700"
                                    >details</Link>
                                </div>
                                <p
                                    v-if="t.answer.answer_preview"
                                    class="mt-1 whitespace-pre-wrap break-words rounded-lg border border-blue-100 bg-blue-50/60 px-3 py-2 text-sm text-gray-900"
                                >{{ t.answer.answer_preview }}</p>
                            </div>

                            <!-- Everything it did in between, folded away. -->
                            <button
                                v-if="t.steps.length"
                                class="text-xs text-gray-500 underline-offset-2 hover:text-gray-900 hover:underline"
                                @click="toggle(t.turn)"
                            >
                                {{ open.has(t.turn) ? 'Hide' : 'Show' }} {{ t.steps.length }}
                                {{ t.steps.length === 1 ? 'step' : 'steps' }} in between
                            </button>

                            <ul v-if="open.has(t.turn)" class="space-y-1">
                                <li
                                    v-for="step in t.steps"
                                    :key="step.id"
                                    class="rounded-md bg-gray-50 px-3 py-2 text-xs"
                                >
                                    <div class="flex items-center gap-2">
                                        <span class="tabular-nums text-gray-400">{{ clock(step.occurred_at) }}</span>
                                        <Tag :label="step.kind" :tone="step.kind === 'utility' ? 'amber' : 'gray'" />
                                        <span class="text-gray-500">{{ step.model ?? 'unknown model' }}</span>
                                        <span class="ml-auto tabular-nums text-gray-400">
                                            {{ count(step.prompt_tokens) }}/{{ count(step.response_tokens) }} tokens
                                        </span>
                                        <Link
                                            :href="route('usage.show', step.id)"
                                            class="text-gray-400 hover:text-gray-700"
                                        >details</Link>
                                    </div>
                                    <p v-if="step.answer_preview" class="mt-1 truncate text-gray-600">
                                        {{ step.answer_preview }}
                                    </p>
                                </li>
                            </ul>
                        </li>

                        <li v-if="!interactions.length" class="px-5 py-10 text-center text-sm text-gray-500">
                            This session has no interactions.
                        </li>
                    </ol>
                </Panel>

                <p class="px-1 text-xs leading-relaxed text-gray-500">
                    <strong class="text-gray-700">What you are reading.</strong> A turn is one message you typed,
                    the reply you got, and the agent's own steps in between. The first lines are shown here;
                    opening the full text is recorded separately in the audit log.
                </p>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
