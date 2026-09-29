<script setup>
import ApplicationLogo from '@/Components/ApplicationLogo.vue';
import Dropdown from '@/Components/Dropdown.vue';
import DropdownLink from '@/Components/DropdownLink.vue';
import ThemeToggle from '@/Components/ThemeToggle.vue';
import { initials } from '@/Components/Usage/format';
import { Link, usePage } from '@inertiajs/vue3';
import { computed, ref } from 'vue';

const open = ref(false);
const user = computed(() => usePage().props.auth.user);
const isManager = computed(() => ['manager', 'admin'].includes(user.value?.role));

// One list for the desktop bar and the phone menu, filtered by role. The
// server enforces the same rules; hiding a link is only tidiness.
const links = computed(() =>
    [
        { label: 'Overview', route: 'usage.index', active: ['usage.index'], show: isManager.value },
        // The list and everything opened from it.
        { label: 'Activity', route: 'usage.activity', active: ['usage.activity', 'usage.session', 'usage.project', 'usage.show'], show: isManager.value },
        { label: 'People', route: 'people.index', active: ['people.*'], show: user.value?.role === 'admin' },
        { label: 'Devices', route: 'pair.show', active: ['pair.*'], show: true },
        { label: 'My data', route: 'usage.my-data', active: ['usage.my-data'], show: true },
    ].filter((l) => l.show),
);
const isActive = (l) => l.active.some((name) => route().current(name));

// Housekeeping, in the user menu rather than next to the pages people read.
const isAdmin = computed(() => user.value?.role === 'admin');

</script>

<template>
    <div class="min-h-screen bg-gray-50">
        <nav class="sticky top-0 z-30 border-b border-gray-200 bg-white/90 backdrop-blur">
            <div class="mx-auto flex h-16 max-w-7xl items-center gap-6 px-4 sm:px-6 lg:px-8">
                <Link :href="route('home')" class="flex shrink-0 items-center gap-2.5">
                    <ApplicationLogo class="h-8 w-8" />
                    <span class="hidden text-sm font-semibold tracking-tight text-gray-900 sm:block">GenAI Log</span>
                </Link>

                <div class="hidden flex-1 items-center gap-1 md:flex">
                    <Link
                        v-for="l in links"
                        :key="l.route"
                        :href="route(l.route)"
                        class="rounded-lg px-3 py-2 text-sm font-medium transition"
                        :class="
                            isActive(l)
                                ? 'bg-gray-100 text-gray-900'
                                : 'text-gray-500 hover:bg-gray-100 hover:text-gray-900'
                        "
                        :aria-current="isActive(l) ? 'page' : undefined"
                    >
                        {{ l.label }}
                    </Link>
                </div>

                <div class="ms-auto flex items-center gap-2">
                    <ThemeToggle />

                    <Dropdown align="right" width="48" class="hidden md:block">
                        <template #trigger>
                            <button
                                type="button"
                                class="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm font-medium text-gray-600 transition hover:bg-gray-100 hover:text-gray-900"
                            >
                                <span class="flex h-7 w-7 items-center justify-center rounded-full bg-indigo-600 text-xs font-semibold text-indigo-50">{{ initials(user.name) }}</span>
                                <span class="max-w-[10rem] truncate">{{ user.name }}</span>
                                <svg class="h-4 w-4 text-gray-400" viewBox="0 0 20 20" fill="currentColor">
                                    <path fill-rule="evenodd" d="M5.3 7.3a1 1 0 011.4 0L10 10.6l3.3-3.3a1 1 0 111.4 1.4l-4 4a1 1 0 01-1.4 0l-4-4a1 1 0 010-1.4z" clip-rule="evenodd" />
                                </svg>
                            </button>
                        </template>
                        <template #content>
                            <div class="border-b border-gray-100 px-4 py-2">
                                <div class="truncate text-sm font-medium text-gray-900">{{ user.name }}</div>
                                <div class="truncate text-xs text-gray-500">{{ user.email }}</div>
                            </div>
                            <DropdownLink :href="route('profile.edit')">Profile</DropdownLink>
                            <DropdownLink v-if="isAdmin" :href="route('usage.deleted')">Deleted prompts</DropdownLink>
                            <DropdownLink :href="route('logout')" method="post" as="button">Log out</DropdownLink>
                        </template>
                    </Dropdown>

                    <button
                        type="button"
                        class="inline-flex h-9 w-9 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 hover:text-gray-900 md:hidden"
                        :aria-expanded="open"
                        aria-label="Menu"
                        @click="open = !open"
                    >
                        <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                            <path v-if="open" d="M6 18L18 6M6 6l12 12" />
                            <path v-else d="M4 6h16M4 12h16M4 18h16" />
                        </svg>
                    </button>
                </div>
            </div>

            <div v-if="open" class="border-t border-gray-200 px-4 pb-4 pt-2 md:hidden">
                <Link
                    v-for="l in links"
                    :key="l.route"
                    :href="route(l.route)"
                    class="block rounded-lg px-3 py-2 text-sm font-medium"
                    :class="isActive(l) ? 'bg-gray-100 text-gray-900' : 'text-gray-600'"
                >
                    {{ l.label }}
                </Link>
                <div class="mt-3 border-t border-gray-200 pt-3">
                    <div class="px-3 text-sm font-medium text-gray-900">{{ user.name }}</div>
                    <div class="px-3 text-xs text-gray-500">{{ user.email }}</div>
                    <Link :href="route('profile.edit')" class="mt-2 block rounded-lg px-3 py-2 text-sm text-gray-600">Profile</Link>
                    <Link v-if="isAdmin" :href="route('usage.deleted')" class="block rounded-lg px-3 py-2 text-sm text-gray-600">Deleted prompts</Link>
                    <Link :href="route('logout')" method="post" as="button" class="block w-full rounded-lg px-3 py-2 text-left text-sm text-gray-600">Log out</Link>
                </div>
            </div>
        </nav>

        <header v-if="$slots.header" class="border-b border-gray-200 bg-white">
            <div class="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
                <slot name="header" />
            </div>
        </header>

        <main>
            <slot />
        </main>
    </div>
</template>
