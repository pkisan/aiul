<script setup>
// The two manager screens, side by side, keeping the chosen period.
import { Link } from '@inertiajs/vue3';

defineProps({ period: Object });
const tabs = [
    { label: 'Team Pulse', route: 'pm.pulse' },
    { label: 'Tools & cost', route: 'pm.tools' },
];
</script>

<template>
    <nav class="inline-flex rounded-lg bg-gray-100 p-0.5 text-sm" aria-label="Manager views">
        <Link
            v-for="t in tabs"
            :key="t.route"
            :href="route(t.route, { period: period.key })"
            class="rounded-md px-3 py-1"
            :class="route().current(t.route) ? 'bg-white font-medium text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-900'"
            :aria-current="route().current(t.route) ? 'page' : undefined"
        >{{ t.label }}</Link>
    </nav>
</template>
