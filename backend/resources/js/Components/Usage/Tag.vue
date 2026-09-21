<script setup>
import { computed } from 'vue';

const props = defineProps({
    label: [String, Number],
    tone: { type: String, default: 'gray' }, // gray | blue | amber | green
});

const tones = {
    gray: 'bg-gray-100 text-gray-700 ring-gray-200',
    blue: 'bg-blue-50 text-blue-700 ring-blue-200',
    amber: 'bg-amber-50 text-amber-800 ring-amber-200',
    green: 'bg-emerald-50 text-emerald-700 ring-emerald-200',
};

const classes = computed(() => tones[props.tone] ?? tones.gray);
</script>

<template>
    <span
        class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset"
        :class="classes"
    >{{ label }}</span>
</template>
