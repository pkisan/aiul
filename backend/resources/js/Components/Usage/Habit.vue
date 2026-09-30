<script setup>
// One dot per person: how many days they used AI (across) against how much they
// used it on those days (up). The dashed lines are the team's medians, so the
// four corners read as "daily and heavy", "daily and light", "in bursts" and
// "now and then". It describes a habit, not how good anyone's work is.
// Plain SVG, no chart library: the page has none and this needs none.
import { initials } from '@/Components/Usage/format';
import { router } from '@inertiajs/vue3';
import { computed } from 'vue';

const props = defineProps({
    people: Array, // team rows: user_id, name, prompts, active_days
    days: Number, // length of the period
    link: Function, // user_id -> Activity URL for that person
});

// Close to the panel's own shape, so 10px text renders at about 10px.
const W = 640;
const H = 250;
const r = 12; // dot radius; the padding keeps a dot on the edge inside the box
const pad = { left: 40, right: 20, top: 18, bottom: 34 };

const median = (values) => {
    const s = [...values].sort((a, b) => a - b);
    const m = Math.floor(s.length / 2);
    return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
};

const dots = computed(() =>
    props.people
        .filter((p) => p.user_id && p.active_days > 0)
        .map((p) => ({ ...p, perDay: Math.round((p.prompts / p.active_days) * 10) / 10 })),
);

// Where each dot is drawn. Days are whole numbers, so people often land on the
// same spot; those after the first step sideways so every one stays readable.
// ponytail: a sideways nudge, not a force layout; fine for a team of tens.
const placed = computed(() => {
    const taken = [];
    return dots.value.map((d) => {
        const cx = x(d.active_days);
        const cy = y(d.perDay);
        const near = taken.filter((t) => Math.abs(t.cx - cx) < 2 * r && Math.abs(t.cy - cy) < 2 * r).length;
        // Alternate left and right of the true position: -1, +1, -2, +2 ...
        const shift = near ? (near % 2 ? -1 : 1) * Math.ceil(near / 2) * (2 * r + 2) : 0;
        const spot = { ...d, cx: Math.min(Math.max(cx + shift, pad.left + r), W - r), cy };
        taken.push({ cx, cy });
        return spot;
    });
});

// Nobody can be active on more days than the period has.
const maxDays = computed(() => Math.max(props.days, 1));
const maxPerDay = computed(() => Math.max(...dots.value.map((d) => d.perDay), 1));
const x = (d) => pad.left + (d / maxDays.value) * (W - pad.left - pad.right);
const y = (v) => H - pad.bottom - (v / maxPerDay.value) * (H - pad.top - pad.bottom);

const medDays = computed(() => median(dots.value.map((d) => d.active_days)));
const medPerDay = computed(() => median(dots.value.map((d) => d.perDay)));

const corner = (d) =>
    d.active_days >= medDays.value
        ? d.perDay >= medPerDay.value ? 'regular, heavy use' : 'regular, light use'
        : d.perDay >= medPerDay.value ? 'uses it in bursts' : 'uses it now and then';
</script>

<template>
    <div class="px-5 py-4">
        <p v-if="dots.length < 2" class="py-10 text-center text-sm text-gray-500">
            Needs at least two people with AI use in this period.
        </p>
        <svg v-else :viewBox="`0 0 ${W} ${H}`" class="w-full" role="img" aria-label="Days with AI use against prompts per active day, one dot per person">
            <!-- Corner names, faint: they orient, the dots are the content -->
            <g class="fill-gray-400 text-[10px]">
                <text :x="x(medDays) + 6" :y="pad.top - 6">regular, heavy</text>
                <text :x="x(medDays) + 6" :y="H - pad.bottom - 6">regular, light</text>
                <text :x="x(medDays) - 6" :y="pad.top - 6" text-anchor="end">in bursts</text>
                <text :x="x(medDays) - 6" :y="H - pad.bottom - 6" text-anchor="end">now and then</text>
            </g>

            <!-- Axes -->
            <line :x1="pad.left" :x2="W - pad.right" :y1="H - pad.bottom" :y2="H - pad.bottom" class="stroke-gray-200" />
            <line :x1="pad.left" :x2="pad.left" :y1="pad.top" :y2="H - pad.bottom" class="stroke-gray-200" />
            <text :x="(W + pad.left) / 2" :y="H - 4" text-anchor="middle" class="fill-gray-500 text-[11px]">days with AI use (of {{ days }})</text>
            <text :x="12" :y="(H - pad.bottom + pad.top) / 2" text-anchor="middle" class="fill-gray-500 text-[11px]" :transform="`rotate(-90 12 ${(H - pad.bottom + pad.top) / 2})`">prompts per day</text>
            <text :x="pad.left" :y="H - pad.bottom + 14" text-anchor="middle" class="fill-gray-400 text-[10px]">0</text>
            <text :x="x(days)" :y="H - pad.bottom + 14" text-anchor="middle" class="fill-gray-400 text-[10px]">{{ days }}</text>
            <text :x="pad.left - 6" :y="pad.top + 4" text-anchor="end" class="fill-gray-400 text-[10px]">{{ maxPerDay }}</text>

            <!-- Team medians -->
            <line :x1="x(medDays)" :x2="x(medDays)" :y1="pad.top" :y2="H - pad.bottom" class="stroke-gray-300" stroke-dasharray="3 3" />
            <line :x1="pad.left" :x2="W - pad.right" :y1="y(medPerDay)" :y2="y(medPerDay)" class="stroke-gray-300" stroke-dasharray="3 3" />

            <!-- People -->
            <g
                v-for="d in placed"
                :key="d.user_id"
                :transform="`translate(${d.cx} ${d.cy})`"
                class="group cursor-pointer"
                role="link"
                tabindex="0"
                :aria-label="`${d.name}, ${corner(d)}`"
                @click="router.visit(link(d.user_id))"
                @keydown.enter="router.visit(link(d.user_id))"
            >
                <title>{{ d.name }}: {{ d.active_days }} day{{ d.active_days === 1 ? '' : 's' }}, {{ d.perDay }} prompts a day ({{ corner(d) }})</title>
                <circle :r="r" class="fill-indigo-100 stroke-indigo-400 transition group-hover:fill-indigo-200 dark:fill-indigo-500/25" />
                <text text-anchor="middle" dy="3.5" class="pointer-events-none fill-indigo-700 text-[10px] font-semibold dark:fill-indigo-200">{{ initials(d.name) }}</text>
            </g>
        </svg>
    </div>
</template>
