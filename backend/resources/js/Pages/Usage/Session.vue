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

        if (row.kind === 'human') turn.asked = row;
        if (row.final_answer) turn.answer = row;
        if (row.kind !== 'human' && !row.final_answer) turn.steps.push(row);
    }

    return [...byTurn.values()];
});

const humanTurns = computed(() => turns.value.filter((t) => t.asked).length);
const utilityCount = computed(() => props.interactions.filter((i) => i.kind === 'utility').length);

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
                    <StatCard label="Agent steps" :value="count(interactions.length - humanTurns - utilityCount)" hint="work done on your behalf" />
                    <StatCard label="Tool's own calls" :value="count(utilityCount)" hint="grading, titles, suggestions" />
                    <StatCard label="AI time" :value="duration(session.seconds)" :hint="when(session.started_at)" />
                </div>

                <Panel title="The session, in order" :subtitle="session.repo ?? undefined">
                    <ol class="divide-y divide-gray-100">
                        <li v-for="t in turns" :key="t.turn" class="px-5 py-4">
                            <!-- What the person typed. -->
                            <div v-if="t.asked" class="flex items-start gap-3">
                                <span class="mt-0.5 w-12 shrink-0 text-xs tabular-nums text-gray-400">
                                    {{ clock(t.asked.occurred_at) }}
                                </span>
                                <div class="min-w-0 flex-1">
                                    <div class="flex items-center gap-2">
                                        <Tag label="you" tone="green" />
                                        <Link :href="route('usage.show', t.asked.id)" class="text-sm font-medium text-gray-900 hover:underline">
                                            Your prompt
                                        </Link>
                                        <span class="text-xs tabular-nums text-gray-400">
                                            {{ count(t.asked.prompt_chars) }} chars
                                        </span>
                                    </div>
                                </div>
                                <Score :value="t.asked.score" class="shrink-0" />
                            </div>
                            <div v-else class="flex items-center gap-2 text-sm text-gray-500">
                                <Tag label="before your first prompt" />
                                <span class="text-xs">work already under way when this session began</span>
                            </div>

                            <!-- What it did about it, folded away by default. -->
                            <div v-if="t.steps.length" class="ml-12 mt-2">
                                <button
                                    class="rounded-md bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-200"
                                    @click="toggle(t.turn)"
                                >
                                    {{ open.has(t.turn) ? 'Hide' : 'Show' }} {{ t.steps.length }} agent
                                    {{ t.steps.length === 1 ? 'step' : 'steps' }}
                                </button>

                                <ul v-if="open.has(t.turn)" class="mt-2 space-y-1">
                                    <li
                                        v-for="step in t.steps"
                                        :key="step.id"
                                        class="flex items-center gap-3 rounded-md bg-gray-50 px-3 py-1.5 text-xs"
                                    >
                                        <span class="w-10 shrink-0 tabular-nums text-gray-400">{{ clock(step.occurred_at) }}</span>
                                        <Tag :label="step.kind" :tone="step.kind === 'utility' ? 'amber' : 'gray'" />
                                        <Link :href="route('usage.show', step.id)" class="text-gray-600 hover:underline">
                                            {{ step.model ?? 'unknown model' }}
                                        </Link>
                                        <span class="ml-auto tabular-nums text-gray-400">
                                            {{ count(step.prompt_tokens) }}/{{ count(step.response_tokens) }} tokens
                                        </span>
                                    </li>
                                </ul>
                            </div>

                            <!-- The answer the person actually read. -->
                            <div
                                v-if="t.answer"
                                class="ml-12 mt-2 flex items-start gap-3 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2"
                            >
                                <Tag label="answer" tone="green" />
                                <Link :href="route('usage.show', t.answer.id)" class="min-w-0 flex-1 text-sm text-emerald-900 hover:underline">
                                    Reply at {{ clock(t.answer.occurred_at) }} · {{ count(t.answer.answer_chars) }} chars
                                </Link>
                            </div>
                        </li>

                        <li v-if="!interactions.length" class="px-5 py-10 text-center text-sm text-gray-500">
                            This session has no interactions.
                        </li>
                    </ol>
                </Panel>

                <p class="px-1 text-xs leading-relaxed text-gray-500">
                    <strong class="text-gray-700">Turns.</strong> A turn is one message you typed, every request the
                    agent made working on it, and the reply it came back with. Rows captured before this distinction
                    existed fall back to the old automated flag and may be grouped wrongly.
                </p>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
