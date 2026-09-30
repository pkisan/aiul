<script setup>
// One headline number and the sentence that says what it means. A number
// without its takeaway is not shown (Phase 4 rule: one takeaway per visual).
import { Link } from '@inertiajs/vue3';

defineProps({ label: String, value: [String, Number], takeaway: String, href: String });
</script>

<template>
    <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
        <p class="text-xs font-medium text-gray-500">{{ label }}</p>
        <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-900">{{ value ?? '—' }}</p>
        <p class="mt-1 text-xs text-gray-600">{{ takeaway }}</p>
        <Link v-if="href" :href="href" class="mt-2 inline-block text-xs font-medium text-indigo-600 hover:underline dark:text-indigo-400"><slot name="link">Details →</slot></Link>
    </div>
</template>
