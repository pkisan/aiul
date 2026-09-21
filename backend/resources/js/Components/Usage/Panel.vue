<script setup>
// One card, one heading, one job. Every list and detail block on the usage pages
// uses this, so the dashboard reads as one surface instead of six.
defineProps({
    title: String,
    subtitle: String,
});
</script>

<template>
    <section class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
        <header
            v-if="title || $slots.actions"
            class="flex items-baseline justify-between gap-4 border-b border-gray-100 px-5 py-4"
        >
            <div>
                <h3 class="text-sm font-semibold text-gray-900">{{ title }}</h3>
                <p v-if="subtitle" class="mt-0.5 text-xs text-gray-500">{{ subtitle }}</p>
            </div>
            <div class="shrink-0 text-xs"><slot name="actions" /></div>
        </header>

        <slot />
    </section>
</template>
