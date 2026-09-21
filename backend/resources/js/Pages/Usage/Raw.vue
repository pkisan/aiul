<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import Tag from '@/Components/Usage/Tag.vue';
import { when } from '@/Components/Usage/format';
import { Head, Link } from '@inertiajs/vue3';
import { computed, ref } from 'vue';

const props = defineProps({
    interaction: Object,
    prompt: String,
    answer: String,
    promptState: String,
    answerState: String,
    auditNotice: String,
});

// An agent re-sends its whole context on every turn, so a prompt is routinely
// tens of thousands of characters. Showing all of it by default buries the part
// a reader came for — the end, where the person's own words are.
const TAIL = 4000;
const showWholePrompt = ref(false);
const promptIsLong = computed(() => (props.prompt?.length ?? 0) > TAIL * 1.5);
const promptShown = computed(() =>
    !promptIsLong.value || showWholePrompt.value ? props.prompt : props.prompt.slice(-TAIL),
);

const copy = (text) => navigator.clipboard?.writeText(text ?? '');
</script>

<template>
    <Head title="Prompt text" />

    <AuthenticatedLayout>
        <template #header>
            <div class="flex items-center gap-3">
                <Link :href="route('usage.show', interaction.id)" class="text-sm text-gray-500 hover:text-gray-900">
                    ← Interaction
                </Link>
                <h2 class="text-xl font-semibold leading-tight text-gray-800">Prompt text</h2>
            </div>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-4xl space-y-4 px-4 sm:px-6 lg:px-8">
                <!-- Said plainly, at the top: this read was recorded. -->
                <div class="rounded-xl border border-amber-200 bg-amber-50 px-5 py-3 text-sm text-amber-900">
                    {{ auditNotice }}
                </div>

                <div class="flex flex-wrap items-center gap-2 rounded-xl border border-gray-200 bg-white px-5 py-3 text-sm">
                    <Tag v-if="interaction.tool" :label="interaction.tool" tone="blue" />
                    <Tag v-if="interaction.model" :label="interaction.model" />
                    <Tag :label="interaction.task_id ?? 'untagged'" :tone="interaction.task_id ? 'green' : 'amber'" />
                    <span class="text-gray-500">{{ when(interaction.occurred_at) }}</span>
                    <span v-if="interaction.redacted?.length" class="ml-auto text-xs text-gray-500">
                        Masked before storage: {{ interaction.redacted.join(', ') }}
                    </span>
                </div>

                <Panel title="Prompt">
                    <template #actions>
                        <button
                            v-if="promptState === 'present'"
                            class="text-gray-500 hover:text-gray-900"
                            @click="copy(prompt)"
                        >Copy</button>
                    </template>

                    <div v-if="promptState === 'present'" class="px-5 py-4">
                        <button
                            v-if="promptIsLong"
                            class="mb-3 rounded-md bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-200"
                            @click="showWholePrompt = !showWholePrompt"
                        >
                            {{ showWholePrompt
                                ? 'Show only the end'
                                : `Show all ${prompt.length.toLocaleString()} characters` }}
                        </button>
                        <p v-if="promptIsLong && !showWholePrompt" class="mb-2 text-xs text-gray-400">
                            Showing the last {{ TAIL.toLocaleString() }} characters — the newest turn is at the end.
                        </p>

                        <pre class="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-4 font-mono text-[13px] leading-relaxed text-gray-800">{{ promptShown }}</pre>
                    </div>
                    <p v-else class="px-5 py-8 text-center text-sm text-gray-500">
                        {{ promptState === 'purged'
                            ? 'This body has been purged by the retention policy.'
                            : 'No prompt text was captured in this exchange.' }}
                    </p>
                </Panel>

                <Panel title="Answer">
                    <template #actions>
                        <button
                            v-if="answerState === 'present'"
                            class="text-gray-500 hover:text-gray-900"
                            @click="copy(answer)"
                        >Copy</button>
                    </template>

                    <div v-if="answerState === 'present'" class="px-5 py-4">
                        <pre class="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-4 font-mono text-[13px] leading-relaxed text-gray-800">{{ answer }}</pre>
                    </div>
                    <p v-else class="px-5 py-8 text-center text-sm text-gray-500">
                        {{ answerState === 'purged'
                            ? 'This body has been purged by the retention policy.'
                            : 'No answer text was captured in this exchange.' }}
                    </p>
                </Panel>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
