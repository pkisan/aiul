<script setup>
// One bar per day (single series). Hover a day for its value; the same
// numbers are in the table below each chart that uses this.
import { computed } from 'vue';

const props = defineProps({
    days: Array, // [{ day: 'YYYY-MM-DD', value: Number }]
    format: { type: Function, default: (v) => v },
    label: String,
});
const max = computed(() => Math.max(0, ...props.days.map((d) => d.value)));
const short = (d) => new Date(d + 'T00:00:00').toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
</script>

<template>
    <div>
        <div class="flex h-28 items-end gap-0.5" role="img" :aria-label="label">
            <div v-for="d in days" :key="d.day" class="group relative flex h-full flex-1 items-end" :title="`${short(d.day)}: ${format(d.value)}`">
                <div
                    class="w-full rounded-t-[4px] bg-indigo-600 transition group-hover:bg-indigo-800 dark:bg-indigo-400 dark:group-hover:bg-indigo-300"
                    :style="{ height: max ? `${Math.max(d.value ? 3 : 0, (d.value / max) * 100)}%` : '0' }"
                ></div>
                <span class="pointer-events-none absolute -top-6 left-1/2 z-10 hidden -translate-x-1/2 whitespace-nowrap rounded bg-gray-900 px-1.5 py-0.5 text-[10px] text-white group-hover:block">{{ short(d.day) }} · {{ format(d.value) }}</span>
            </div>
        </div>
        <div class="mt-1 flex justify-between border-t border-gray-200 pt-1 text-[10px] text-gray-500">
            <span>{{ days.length ? short(days[0].day) : '' }}</span>
            <span>{{ days.length ? short(days[days.length - 1].day) : '' }}</span>
        </div>
    </div>
</template>
