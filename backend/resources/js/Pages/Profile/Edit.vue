<script setup>
import AuthenticatedLayout from '@/Layouts/AuthenticatedLayout.vue';
import UpdatePasswordForm from './Partials/UpdatePasswordForm.vue';
import UpdateProfileInformationForm from './Partials/UpdateProfileInformationForm.vue';
import { Head } from '@inertiajs/vue3';

defineProps({
    mustVerifyEmail: Boolean,
    status: String,
});
</script>

<template>
    <Head title="Profile" />

    <AuthenticatedLayout>
        <div class="mx-auto max-w-3xl space-y-6 px-4 py-8 sm:px-6 lg:px-8">
            <h1 class="text-xl font-semibold text-gray-900">Profile</h1>

            <!-- No "delete my account" here: deleting a user erases their
                 consent records. An admin can reset a password. -->
            <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
                <UpdateProfileInformationForm :must-verify-email="mustVerifyEmail" :status="status" class="max-w-xl" />
            </div>

            <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
                <UpdatePasswordForm class="max-w-xl" />
            </div>
        </div>
    </AuthenticatedLayout>
</template>
