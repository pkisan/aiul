import { defineConfig } from 'vite';
import laravel from 'laravel-vite-plugin';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
    // pm-tool/ (the PM module) sits outside this folder: let the dev server read
    // it, and resolve its imports of vue and inertia from this node_modules.
    server: { fs: { allow: ['..'] } },
    resolve: { dedupe: ['vue', '@inertiajs/vue3'] },
    plugins: [
        laravel({
            input: 'resources/js/app.js',
            refresh: true,
        }),
        vue({
            template: {
                transformAssetUrls: {
                    base: null,
                    includeAbsolute: false,
                },
            },
        }),
    ],
});
