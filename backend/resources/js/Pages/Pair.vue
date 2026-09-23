<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import Panel from '@/Components/Usage/Panel.vue';
import { when } from '@/Components/Usage/format';
import { Head, useForm } from '@inertiajs/vue3';

defineProps({ devices: Array, status: String });

// Typed by hand, never taken from a link: a link carrying someone else's code
// would let them attach their device to your account with one click.
const form = useForm({ code: '' });
</script>

<template>
    <Head title="Link a device" />

    <AuthenticatedLayout>
        <template #header>
            <h2 class="text-xl font-semibold leading-tight text-gray-800">Link a device</h2>
        </template>

        <div class="bg-gray-50 py-8">
            <div class="mx-auto max-w-xl space-y-6 px-4 sm:px-6 lg:px-8">
                <Panel title="Enter the code" subtitle="Run  aiul login  on the device; it prints an 8-character code">
                    <form class="flex items-start gap-3 px-5 py-4" @submit.prevent="form.post(route('pair.store'), { onSuccess: () => form.reset() })">
                        <div class="flex-1">
                            <input
                                v-model="form.code"
                                type="text"
                                placeholder="ABCD-2345"
                                autocomplete="off"
                                class="w-full rounded-lg border-gray-300 font-mono uppercase tracking-widest"
                            />
                            <p v-if="form.errors.code" class="mt-1 text-sm text-rose-600">{{ form.errors.code }}</p>
                            <p v-if="status" class="mt-1 text-sm text-emerald-700">{{ status }}</p>
                        </div>
                        <button
                            type="submit"
                            :disabled="form.processing || !form.code"
                            class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-700 disabled:opacity-40"
                        >Link</button>
                    </form>
                </Panel>

                <Panel title="Your devices">
                    <ul class="divide-y divide-gray-100">
                        <li v-for="d in devices" :key="d.id" class="flex justify-between px-5 py-3 text-sm">
                            <span class="font-medium text-gray-900">{{ d.hostname }}</span>
                            <span class="text-gray-500">{{ d.platform }} · last seen {{ when(d.last_seen_at) }}</span>
                        </li>
                        <li v-if="!devices.length" class="px-5 py-6 text-center text-sm text-gray-500">No devices linked yet.</li>
                    </ul>
                </Panel>
            </div>
        </div>
    </AuthenticatedLayout>
</template>
