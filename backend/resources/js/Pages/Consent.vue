<script setup>
import GuestLayout from '@/Layouts/GuestLayout.vue';
import { Head, Link, useForm } from '@inertiajs/vue3';

defineProps({ version: String, points: Array });

const form = useForm({ accept: false });
</script>

<template>
    <Head title="Before you start" />

    <GuestLayout>
        <h1 class="text-lg font-semibold text-gray-900">Before you start</h1>
        <p class="mt-1 text-sm text-gray-600">
            This company records how AI tools are used on its devices. Please read what that means.
        </p>

        <ul class="mt-4 list-disc space-y-2 pl-5 text-sm text-gray-700">
            <li v-for="p in points" :key="p">{{ p }}</li>
        </ul>

        <form class="mt-6 space-y-4" @submit.prevent="form.post(route('consent.store'))">
            <label class="flex items-start gap-2 text-sm text-gray-800">
                <input v-model="form.accept" type="checkbox" class="mt-0.5 rounded border-gray-300" />
                I have read this and agree to AI usage on my company devices being recorded.
            </label>
            <p v-if="form.errors.accept" class="text-sm text-rose-600">Please tick the box to continue.</p>

            <div class="flex items-center justify-between">
                <Link :href="route('logout')" method="post" as="button" class="text-sm text-gray-500 hover:text-gray-900">
                    Log out
                </Link>
                <button
                    type="submit"
                    :disabled="!form.accept || form.processing"
                    class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-700 disabled:opacity-40"
                >Accept and continue</button>
            </div>
            <p class="text-xs text-gray-400">Notice version {{ version }}</p>
        </form>
    </GuestLayout>
</template>
