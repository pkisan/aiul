<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import { count, when } from '@/Components/Usage/format';
import { Head } from '@inertiajs/vue3';

defineProps({
    summary: Object,
    explanation: Array,
});
</script>

<template>
    <Head title="My data" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-3xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <h1 class="text-2xl font-semibold tracking-tight text-gray-900">My data</h1>

            <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
                <h2 class="text-sm font-semibold text-gray-900">What is captured</h2>
                <ul class="mt-3 list-disc space-y-1.5 pl-5 text-sm text-gray-600">
                    <li v-for="line in explanation" :key="line">{{ line }}</li>
                </ul>
            </div>

            <div class="grid gap-4 sm:grid-cols-3">
                <div class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
                    <div class="text-xs font-medium uppercase tracking-wide text-gray-500">Interactions captured</div>
                    <div class="mt-1 text-2xl font-semibold tabular-nums text-gray-900">{{ count(summary.total) }}</div>
                </div>
                <div class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
                    <div class="text-xs font-medium uppercase tracking-wide text-gray-500">Since</div>
                    <div class="mt-1 text-base font-semibold text-gray-900">{{ when(summary.first_seen) }}</div>
                </div>
                <div class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
                    <div class="text-xs font-medium uppercase tracking-wide text-gray-500">My devices</div>
                    <div class="mt-1 text-2xl font-semibold tabular-nums text-gray-900">{{ summary.devices.length }}</div>
                    <div v-for="device in summary.devices" :key="device.id" class="truncate text-xs text-gray-400">
                        {{ device.hostname }}
                    </div>
                </div>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
