<script setup>
defineProps({
    label: String,
    value: [String, Number],
    hint: String,
});
</script>

<template>
    <div class="rounded-xl border border-gray-200 bg-white px-5 py-4 shadow-sm">
        <div class="text-xs font-medium uppercase tracking-wide text-gray-500">{{ label }}</div>
        <div class="mt-1 text-2xl font-semibold tabular-nums text-gray-900">{{ value }}</div>
        <div v-if="hint" class="mt-0.5 text-xs text-gray-400">{{ hint }}</div>
    </div>
</template>
