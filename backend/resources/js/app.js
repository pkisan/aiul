import '../css/app.css';
import './bootstrap';

import { createInertiaApp } from '@inertiajs/vue3';
import { resolvePageComponent } from 'laravel-vite-plugin/inertia-helpers';
import { createApp, h } from 'vue';
import { ZiggyVue } from '../../vendor/tightenco/ziggy';

const appName = import.meta.env.VITE_APP_NAME || 'GenAI Log';

createInertiaApp({
    title: (title) => `${title} - ${appName}`,
    // Pm/* pages live in the PM module, pm-tool/ beside backend/ (D19).
    resolve: (name) =>
        name.startsWith('Pm/')
            ? resolvePageComponent(`../../../pm-tool/resources/js/Pages/${name}.vue`, import.meta.glob('../../../pm-tool/resources/js/Pages/**/*.vue'))
            : resolvePageComponent(`./Pages/${name}.vue`, import.meta.glob('./Pages/**/*.vue')),
    setup({ el, App, props, plugin }) {
        return createApp({ render: () => h(App, props) })
            .use(plugin)
            .use(ZiggyVue)
            .mount(el);
    },
    progress: {
        color: '#4B5563',
    },
});
