import defaultTheme from 'tailwindcss/defaultTheme';
import forms from '@tailwindcss/forms';

// Grey and white are CSS variables (set in resources/css/app.css) that swap
// values under `.dark`. Every page is built from these neutrals, so every page
// gets a dark mode without a `dark:` twin on each class. Accent colours
// (indigo, amber, ...) are not swapped: where one needs a dark variant it says
// so with `dark:`.
const neutral = (name) => `rgb(var(--${name}) / <alpha-value>)`;

/** @type {import('tailwindcss').Config} */
export default {
    darkMode: 'class',

    content: [
        './vendor/laravel/framework/src/Illuminate/Pagination/resources/views/*.blade.php',
        './storage/framework/views/*.php',
        './resources/views/**/*.blade.php',
        './resources/js/**/*.vue',
        '../pm-tool/resources/js/**/*.vue',
    ],

    theme: {
        extend: {
            colors: {
                white: neutral('white'),
                gray: Object.fromEntries(
                    [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950].map((n) => [n, neutral(`gray-${n}`)]),
                ),
            },
            fontFamily: {
                sans: ['Figtree', ...defaultTheme.fontFamily.sans],
            },
        },
    },

    plugins: [forms],
};
