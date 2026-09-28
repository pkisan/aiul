<script setup>
import ApplicationLogo from '@/Components/ApplicationLogo.vue';
import ThemeToggle from '@/Components/ThemeToggle.vue';
</script>

<template>
    <div class="relative flex min-h-screen flex-col items-center justify-center bg-gray-50 px-4 py-10">
        <ThemeToggle class="absolute right-4 top-4" />

        <div class="flex flex-col items-center gap-3">
            <ApplicationLogo class="h-14 w-14" />
            <h1 class="text-lg font-semibold tracking-tight text-gray-900">GenAI Log</h1>
        </div>

        <div class="mt-8 w-full rounded-2xl border border-gray-200 bg-white px-6 py-6 shadow-sm sm:max-w-md">
            <slot />
        </div>
    </div>
</template>
