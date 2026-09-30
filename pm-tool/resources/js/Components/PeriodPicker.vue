<script setup>
import { router } from '@inertiajs/vue3';

const props = defineProps({ period: Object, periods: Array });
const pick = (key) => router.get(window.location.pathname, { period: key }, { preserveScroll: true });
</script>

<template>
    <label class="text-xs font-medium text-gray-500">
        Period
        <select :value="period.key" class="mt-1 block rounded-lg py-1.5 text-sm" @change="pick($event.target.value)">
            <option v-for="p in periods" :key="p.key" :value="p.key">{{ p.label }}</option>
        </select>
    </label>
</template>
