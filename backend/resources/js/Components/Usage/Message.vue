<script setup>
import { computed, ref } from 'vue';

// One chat bubble. Who spoke decides everything about how it looks, so a reader
// can tell a person's words from the model's without reading a label:
//
//   you       — what the person typed: right-aligned, dark
//   assistant — the reply they read: left-aligned, white
//   agent     — the agent feeding itself a tool result: small, grey, indented
//   tool      — the tool's own housekeeping call: small, amber, indented
const props = defineProps({
    who: { type: String, default: 'assistant' },
    text: String,
    placeholder: String,
    // Long text is folded; the whole of it is one click away.
    fold: { type: Number, default: 1500 },
});

const open = ref(false);
const long = computed(() => (props.text?.length ?? 0) > props.fold);
const shown = computed(() => (long.value && !open.value ? props.text.slice(0, props.fold) + '…' : props.text));

const styles = {
    you: 'ml-auto max-w-[85%] rounded-2xl rounded-br-sm bg-gray-900 text-white px-4 py-3 text-sm',
    assistant: 'mr-auto max-w-[85%] rounded-2xl rounded-bl-sm border border-gray-200 bg-white text-gray-900 px-4 py-3 text-sm shadow-sm',
    agent: 'mr-auto ml-6 max-w-[80%] rounded-lg border border-dashed border-gray-300 bg-gray-50 text-gray-600 px-3 py-2 text-xs',
    tool: 'mr-auto ml-6 max-w-[80%] rounded-lg border border-dashed border-amber-200 bg-amber-50/60 text-amber-900 px-3 py-2 text-xs',
};
</script>

<template>
    <div :class="styles[who] ?? styles.assistant">
        <div v-if="$slots.meta" class="mb-1 flex flex-wrap items-center gap-2 text-[11px] opacity-70">
            <slot name="meta" />
        </div>
        <p v-if="text" class="whitespace-pre-wrap break-words leading-relaxed">{{ shown }}</p>
        <p v-else class="italic opacity-60">{{ placeholder }}</p>
        <button v-if="long" class="mt-1 text-[11px] underline opacity-70 hover:opacity-100" @click="open = !open">
            {{ open ? 'Show less' : `Show all ${text.length.toLocaleString()} characters` }}
        </button>
    </div>
</template>
