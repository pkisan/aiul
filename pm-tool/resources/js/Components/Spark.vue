<script setup>
// A tiny single-series trend: sessions per day. 2px line, a dot on the last
// day, hover on the whole line gives the numbers.
import { computed } from 'vue';

const props = defineProps({ values: Array, days: Array, label: String });
const W = 96;
const H = 24;
const max = computed(() => Math.max(1, ...props.values));
const x = (i) => (props.values.length < 2 ? W / 2 : (i * (W - 4)) / (props.values.length - 1) + 2);
const y = (v) => H - 3 - (v / max.value) * (H - 6);
const points = computed(() => props.values.map((v, i) => `${x(i)},${y(v)}`).join(' '));
const title = computed(() => `${props.label}: ${props.values.reduce((a, b) => a + b, 0)} sessions, most in a day ${Math.max(0, ...props.values)}`);
</script>

<template>
    <svg :width="W" :height="H" :viewBox="`0 0 ${W} ${H}`" role="img" :aria-label="title" class="overflow-visible">
        <title>{{ title }}</title>
        <line :x1="0" :x2="W" :y1="H - 3" :y2="H - 3" class="stroke-gray-200" stroke-width="1" />
        <polyline v-if="values.length > 1" :points="points" fill="none" class="stroke-indigo-600 dark:stroke-indigo-400" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
        <circle v-if="values.length" :cx="x(values.length - 1)" :cy="y(values[values.length - 1])" r="2.5" class="fill-indigo-600 dark:fill-indigo-400" />
    </svg>
</template>
