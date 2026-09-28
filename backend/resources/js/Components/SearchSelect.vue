<script setup>
import { computed, nextTick, onBeforeUnmount, ref } from 'vue';

// A dropdown with a search box: type to narrow the list, arrows to move,
// Enter to pick, Escape to close. For lists that grow (people, projects);
// a native <select> is still right for short fixed ones.
//
// options: [{ value, label, title? }]. Values are compared as strings, so a
// numeric id from the URL matches the same id from the database.
const props = defineProps({
    modelValue: { type: [String, Number], default: '' },
    options: { type: Array, required: true },
    label: { type: String, required: true }, // for screen readers
});
const emit = defineEmits(['update:modelValue']);

const open = ref(false);
const query = ref('');
const active = ref(0);
const root = ref(null);
const input = ref(null);
const list = ref(null);

const same = (a, b) => String(a ?? '') === String(b ?? '');
const selected = computed(() => props.options.find((o) => same(o.value, props.modelValue)) ?? props.options[0]);
const shown = computed(() => {
    const q = query.value.trim().toLowerCase();
    return q ? props.options.filter((o) => o.label.toLowerCase().includes(q)) : props.options;
});

function show() {
    open.value = true;
    query.value = '';
    active.value = Math.max(0, props.options.indexOf(selected.value));
    document.addEventListener('mousedown', outside);
    nextTick(() => {
        input.value?.focus();
        scrollToActive();
    });
}
function hide() {
    open.value = false;
    document.removeEventListener('mousedown', outside);
}
const outside = (e) => root.value?.contains(e.target) || hide();
onBeforeUnmount(hide);

function pick(option) {
    hide();
    if (option && !same(option.value, props.modelValue)) emit('update:modelValue', option.value);
}
function move(step) {
    if (!shown.value.length) return;
    active.value = (active.value + step + shown.value.length) % shown.value.length;
    scrollToActive();
}
const scrollToActive = () => nextTick(() => list.value?.children[active.value]?.scrollIntoView({ block: 'nearest' }));
</script>

<template>
    <div ref="root" class="relative">
        <button
            type="button"
            class="flex w-full items-center justify-between gap-2 rounded-lg border border-gray-300 bg-white py-1.5 pl-3 pr-2 text-left text-sm text-gray-900 shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            :aria-label="label"
            aria-haspopup="listbox"
            :aria-expanded="open"
            :title="selected?.title"
            @click="open ? hide() : show()"
        >
            <span class="truncate">{{ selected?.label }}</span>
            <svg class="h-4 w-4 shrink-0 text-gray-400" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.17l3.71-3.94a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
            </svg>
        </button>

        <div v-if="open" class="absolute left-0 z-40 mt-1 w-64 max-w-[calc(100vw-2rem)] rounded-lg border border-gray-200 bg-white shadow-lg">
            <div class="border-b border-gray-100 p-2">
                <input
                    ref="input"
                    v-model="query"
                    type="search"
                    :placeholder="`Search ${label.toLowerCase()}…`"
                    class="block w-full rounded-md py-1 text-sm"
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-label="`Search ${label}`"
                    @input="active = 0"
                    @keydown.down.prevent="move(1)"
                    @keydown.up.prevent="move(-1)"
                    @keydown.enter.prevent="pick(shown[active])"
                    @keydown.esc.prevent="hide()"
                    @keydown.tab="hide()"
                />
            </div>
            <ul ref="list" role="listbox" :aria-label="label" class="max-h-64 overflow-y-auto py-1">
                <li
                    v-for="(o, i) in shown"
                    :key="String(o.value)"
                    role="option"
                    :aria-selected="same(o.value, modelValue)"
                    :title="o.title"
                    class="cursor-pointer truncate px-3 py-1.5 text-sm"
                    :class="[
                        i === active ? 'bg-gray-100 text-gray-900' : 'text-gray-700',
                        same(o.value, modelValue) ? 'font-medium' : '',
                    ]"
                    @mouseenter="active = i"
                    @mousedown.prevent="pick(o)"
                >{{ o.label }}</li>
                <li v-if="!shown.length" class="px-3 py-1.5 text-sm text-gray-400">No matches</li>
            </ul>
        </div>
    </div>
</template>
