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

// One layout for every session. Where the agent recorded who caused a request,
// the row is labelled; where it did not — anything captured before kinds
// existed — the row simply carries no label. Two different pages for the same
// URL was worse than either.
const labelled = computed(() => props.interactions.some((i) => !i.legacy_kind));

const tone = (kind) => ({ human: 'green', agent: 'gray', utility: 'amber' })[kind] ?? 'gray';
const label = (kind) => ({ human: 'you asked', agent: 'agent step', utility: "tool's own call" })[kind] ?? kind;

// Exchanges worth reading first: something came back that is more than a token
// or two. The rest are the agent's mechanics and sit behind the toggle.
const substantial = computed(() => props.interactions.filter((i) => (i.answer_chars ?? 0) > 40));
const showAll = ref(false);
const listed = computed(() => (showAll.value ? props.interactions : substantial.value));

const humanTurns = computed(() => props.interactions.filter((i) => i.kind === 'human' && !i.legacy_kind).length);
const utilityCount = computed(() => props.interactions.filter((i) => i.kind === 'utility' && !i.legacy_kind).length);
const tokens = computed(() =>
    props.interactions.reduce((n, i) => n + (i.prompt_tokens ?? 0) + (i.response_tokens ?? 0), 0),
);
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
                    <StatCard
                        v-if="labelled"
                        label="You asked"
                        :value="count(humanTurns)"
                        hint="turns you typed"
                    />
                    <StatCard v-else label="Exchanges" :value="count(interactions.length)" hint="requests in this session" />
                    <StatCard label="With a reply" :value="count(substantial.length)" hint="more than a token or two back" />
                    <StatCard
                        v-if="labelled"
                        label="Tool's own calls"
                        :value="count(utilityCount)"
                        hint="grading, titles, suggestions"
                    />
                    <StatCard v-else label="Tokens" :value="count(tokens)" />
                    <StatCard label="AI time" :value="duration(session.seconds)" :hint="when(session.started_at)" />
                </div>

                <Panel title="Exchanges, in order" :subtitle="session.repo ?? undefined">
                    <template #actions>
                        <button class="text-gray-500 hover:text-gray-900" @click="showAll = !showAll">
                            {{ showAll ? 'Only ones with a reply' : `Show all ${interactions.length}` }}
                        </button>
                    </template>

                    <ul class="divide-y divide-gray-100">
                        <li v-for="i in listed" :key="i.id" class="px-5 py-4">
                            <div class="flex flex-wrap items-center gap-2 text-xs text-gray-400">
                                <span class="tabular-nums">{{ clock(i.occurred_at) }}</span>
                                <Tag v-if="!i.legacy_kind" :label="label(i.kind)" :tone="tone(i.kind)" />
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
                            <p v-else-if="!canViewRaw" class="mt-2 text-xs italic text-gray-400">
                                You do not have permission to read prompt text.
                            </p>

                            <p
                                v-if="i.answer_preview"
                                class="mt-1 whitespace-pre-wrap break-words rounded-lg border border-blue-100 bg-blue-50/60 px-3 py-2 text-sm text-gray-900"
                            >{{ i.answer_preview }}</p>
                            <p v-else-if="canViewRaw" class="mt-1 text-xs italic text-gray-400">No reply text captured.</p>
                        </li>

                        <li v-if="!listed.length" class="px-5 py-10 text-center text-sm text-gray-500">
                            Nothing with text in this session.
                        </li>
                    </ul>
                </Panel>

                <p class="px-1 text-xs leading-relaxed text-gray-500">
                    <template v-if="labelled">
                        <strong class="text-gray-700">Labels.</strong> "you asked" is a message you typed, "agent step"
                        is the agent continuing that work on its own, and "tool's own call" is the tool talking to a
                        model for itself — grading a prompt, naming a conversation.
                    </template>
                    <template v-else>
                        <strong class="text-gray-700">No labels here.</strong> This session was captured before the
                        agent could tell your prompts from its own follow-ups, so the exchanges are listed in order
                        and unlabelled.
                    </template>
                    Opening the full text of an exchange is recorded in the audit log.
                </p>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
