<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import StatCard from '@/Components/Usage/StatCard.vue';
import Score from '@/Components/Usage/Score.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { clock, count, duration, when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';

defineProps({
    session: Object,
    interactions: Array,
    canViewRaw: Boolean,
});
</script>

<template>
    <Head :title="`Session ${session.id}`" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.index')" class="text-sm text-gray-500 hover:text-gray-900">← AI usage</Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">
                    {{ session.task_id ?? 'Untagged session' }}
                </h2>
                <Tag v-if="session.tool" :label="session.tool" tone="blue" />
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-5xl space-y-6 px-4 sm:px-6 lg:px-8">
                <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
                    <StatCard label="Interactions" :value="count(interactions.length)" />
                    <StatCard
                        label="Human prompts"
                        :value="count(interactions.filter((i) => !i.automated).length)"
                    />
                    <StatCard label="AI time" :value="duration(session.seconds)" />
                    <StatCard label="Started" :value="when(session.started_at)" :hint="session.branch ?? undefined" />
                </div>

                <!-- Oldest first: a session is a conversation, and a conversation
                     reads forwards. -->
                <Panel title="The session, in order" :subtitle="session.repo ?? undefined">
                    <ol class="divide-y divide-gray-100">
                        <li
                            v-for="i in interactions"
                            :key="i.id"
                            class="flex items-center gap-4 px-5 py-3 hover:bg-gray-50"
                        >
                            <span class="w-14 shrink-0 text-xs tabular-nums text-gray-400">{{ clock(i.occurred_at) }}</span>

                            <div class="min-w-0 flex-1">
                                <Link :href="route('usage.show', i.id)" class="text-sm font-medium text-gray-900 hover:underline">
                                    {{ i.automated ? 'Automated follow-up' : 'Prompt' }}
                                </Link>
                                <div class="mt-0.5 flex items-center gap-2 text-xs text-gray-500">
                                    <Tag v-if="i.model" :label="i.model" />
                                    <span class="tabular-nums">
                                        {{ count(i.prompt_chars) }} in / {{ count(i.answer_chars) }} out chars
                                    </span>
                                    <span v-if="i.response_tokens" class="tabular-nums">
                                        · {{ count(i.prompt_tokens) }}/{{ count(i.response_tokens) }} tokens
                                    </span>
                                </div>
                            </div>

                            <Score :value="i.score" class="shrink-0" />
                        </li>

                        <li v-if="!interactions.length" class="px-5 py-10 text-center text-sm text-gray-500">
                            This session has no interactions.
                        </li>
                    </ol>
                </Panel>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
