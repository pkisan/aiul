<template>
    <!-- Two files rather than one SVG with a colour-scheme media query: the
         media query follows the OS, and the logo has to follow the app's own
         light/dark toggle. -->
    <span class="inline-flex">
        <img src="/logo-light.svg" alt="Aayatti" class="h-full w-auto dark:hidden" />
        <img src="/logo-dark.svg" alt="Aayatti" class="hidden h-full w-auto dark:block" />
    </span>
</template>
