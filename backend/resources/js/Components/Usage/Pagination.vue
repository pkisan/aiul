<script setup>
import { Link } from '@inertiajs/vue3';

// Footer for any Laravel paginator: "Showing 26–50 of 615", then Previous,
// page numbers, Next. The links already carry the page's filters
// (withQueryString on the server).
defineProps({
    page: { type: Object, required: true },
    noun: { type: String, default: 'items' },
});

// Laravel's first and last links are "&laquo; Previous" / "Next &raquo;":
// drawn here as arrows rather than trusted as HTML.
const label = (link, i, all) => (i === 0 ? '‹' : i === all.length - 1 ? '›' : link.label);
const aria = (i, all, link) => (i === 0 ? 'Previous page' : i === all.length - 1 ? 'Next page' : `Page ${link.label}`);
</script>

<template>
    <div
        v-if="page.total > 0"
        class="flex flex-col items-center justify-between gap-3 border-t border-gray-100 px-5 py-3 text-sm sm:flex-row"
    >
        <p class="text-gray-500">
            Showing <span class="font-medium text-gray-900">{{ page.from }}–{{ page.to }}</span> of
            <span class="font-medium text-gray-900">{{ page.total.toLocaleString() }}</span> {{ noun }}
        </p>

        <nav v-if="page.last_page > 1" class="flex items-center gap-1" aria-label="Pagination">
            <template v-for="(link, i) in page.links" :key="i">
                <span v-if="link.label === '...'" class="px-2 text-gray-400">…</span>
                <Link
                    v-else-if="link.url"
                    :href="link.url"
                    preserve-scroll
                    :aria-label="aria(i, page.links, link)"
                    :aria-current="link.active ? 'page' : undefined"
                    class="inline-flex h-8 min-w-8 items-center justify-center rounded-lg px-2 tabular-nums transition"
                    :class="link.active ? 'bg-indigo-600 font-medium text-indigo-50' : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'"
                >{{ label(link, i, page.links) }}</Link>
                <span
                    v-else
                    class="inline-flex h-8 min-w-8 items-center justify-center px-2 text-gray-300"
                    aria-hidden="true"
                >{{ label(link, i, page.links) }}</span>
            </template>
        </nav>
    </div>
</template>
