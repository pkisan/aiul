<script setup>
import { computed } from 'vue';

const props = defineProps({ value: [Number, String, null] });

// A score is a number with a judgement attached, so it gets a colour. An absent
// score is NOT a zero: automated follow-ups are never scored, and a dash says so.
const tone = computed(() => {
    if (props.value === null || props.value === undefined) return 'text-gray-300';
    if (props.value >= 75) return 'text-emerald-600';
    if (props.value >= 50) return 'text-amber-600';
    return 'text-rose-600';
});
</script>

<template>
    <span class="font-semibold tabular-nums" :class="tone">
        {{ value === null || value === undefined ? '—' : value }}
    </span>
</template>
