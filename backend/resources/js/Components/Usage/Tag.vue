<script setup>
import { computed } from 'vue';

const props = defineProps({
    label: [String, Number],
    tone: { type: String, default: 'gray' }, // gray | blue | amber | green
});

const tones = {
    gray: 'bg-gray-100 text-gray-700 ring-gray-200',
    blue: 'bg-indigo-50 text-indigo-700 ring-indigo-200 dark:bg-indigo-500/15 dark:text-indigo-300 dark:ring-indigo-500/30',
    amber: 'bg-amber-50 text-amber-800 ring-amber-200 dark:bg-amber-500/15 dark:text-amber-300 dark:ring-amber-500/30',
    green: 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-500/15 dark:text-emerald-300 dark:ring-emerald-500/30',
};

const classes = computed(() => tones[props.tone] ?? tones.gray);
</script>

<template>
    <span
        class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset"
        :class="classes"
    >{{ label }}</span>
</template>
