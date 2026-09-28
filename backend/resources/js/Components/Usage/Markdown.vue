<script setup>
import DOMPurify from 'dompurify';
import hljs from 'highlight.js/lib/common';
import 'highlight.js/styles/github.css';
import { Marked } from 'marked';
import { computed } from 'vue';

// An AI answer, drawn the way the person saw it: bold, lists, tables, code.
//
// The text is UNTRUSTED — it is whatever a model wrote, and a model can be
// talked into writing HTML. So two guards, both needed:
//   1. raw HTML in the markdown is shown as text, never as markup (an answer
//      that explains <script> shows the tag, it does not run it);
//   2. the finished HTML goes through DOMPurify, which removes anything that
//      could run, including javascript: links.
const props = defineProps({ text: String });

const escape = (s) =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

const marked = new Marked({
    gfm: true,
    breaks: true, // chat answers use single newlines as line breaks
    renderer: {
        html: ({ text }) => escape(text),
        code({ text, lang }) {
            const language = (lang || '').trim().split(/\s+/)[0];
            const known = language && hljs.getLanguage(language);
            const body = known
                ? hljs.highlight(text, { language, ignoreIllegals: true }).value
                : escape(text);
            return (
                '<div class="md-code">' +
                `<div class="md-code-bar"><span>${escape(language || 'text')}</span>` +
                '<button type="button" data-copy>Copy</button></div>' +
                `<pre><code class="hljs">${body}</code></pre></div>`
            );
        },
    },
});

// Links open in a new tab and tell the site nothing about where they came from.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A') {
        node.setAttribute('target', '_blank');
        node.setAttribute('rel', 'noopener noreferrer');
    }
});

const html = computed(() => DOMPurify.sanitize(marked.parse(props.text ?? '')));

// One listener for every Copy button in the answer.
function onClick(event) {
    const button = event.target.closest('[data-copy]');
    if (!button) return;
    const code = button.closest('.md-code')?.querySelector('code')?.textContent ?? '';
    navigator.clipboard?.writeText(code).then(() => {
        button.textContent = 'Copied';
        setTimeout(() => (button.textContent = 'Copy'), 1500);
    });
}
</script>

<template>
    <div class="md" v-html="html" @click="onClick" />
</template>

<style>
/* Answer typography. A serif body sets the model's words apart from the
   interface around them, as claude.ai does; code stays monospace.
   Text, rules and tables use the --gray-* variables so they flip in dark mode.
   Code (blocks and inline) keeps its own light background on purpose: the
   highlight.js theme is a light one, so it stays readable in both modes. */
.md {
    font-family: ui-serif, Georgia, Cambria, 'Times New Roman', serif;
    font-size: 1rem;
    line-height: 1.7;
    color: rgb(var(--gray-900));
    overflow-wrap: anywhere;
}
.md > :first-child { margin-top: 0; }
.md > :last-child { margin-bottom: 0; }
.md p, .md ul, .md ol, .md blockquote, .md table, .md .md-code { margin: 0.9em 0; }
.md h1, .md h2, .md h3, .md h4 { font-weight: 600; line-height: 1.3; margin: 1.4em 0 0.5em; }
.md h1 { font-size: 1.5rem; }
.md h2 { font-size: 1.3rem; }
.md h3 { font-size: 1.125rem; }
.md h4 { font-size: 1rem; }
.md strong { font-weight: 700; }
.md ul { list-style: disc; padding-left: 1.6em; }
.md ol { list-style: decimal; padding-left: 1.6em; }
.md li { margin: 0.3em 0; padding-left: 0.2em; }
.md li > p { margin: 0.3em 0; }
.md a { color: rgb(37 99 235); text-decoration: underline; text-underline-offset: 2px; }
.md blockquote { border-left: 3px solid rgb(var(--gray-300)); padding-left: 1em; color: rgb(var(--gray-600)); }
.md hr { border: 0; border-top: 1px solid rgb(var(--gray-200)); margin: 1.5em 0; }
.md :not(pre) > code {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.85em;
    color: rgb(185 28 28);
    background: rgb(243 244 246);
    border: 1px solid rgb(229 231 235);
    border-radius: 0.3rem;
    padding: 0.1em 0.35em;
}
.md table { border-collapse: collapse; font-size: 0.9rem; display: block; overflow-x: auto; }
.md th, .md td { border: 1px solid rgb(var(--gray-200)); padding: 0.4em 0.75em; text-align: left; }
.md th { background: rgb(var(--gray-50)); font-weight: 600; }
.dark .md a { color: rgb(96 165 250); }
.md-code { border: 1px solid rgb(229 231 235); border-radius: 0.6rem; background: rgb(250 250 250); color: rgb(17 24 39); overflow: hidden; }
.md-code-bar {
    display: flex; justify-content: space-between; align-items: center;
    padding: 0.4rem 0.9rem;
    font-family: ui-sans-serif, system-ui, sans-serif; font-size: 0.75rem; color: rgb(107 114 128);
}
.md-code-bar button { color: rgb(107 114 128); }
.md-code-bar button:hover { color: rgb(17 24 39); }
.md-code pre { margin: 0; padding: 0.25rem 0.9rem 0.9rem; overflow-x: auto; }
.md-code code.hljs {
    background: transparent; padding: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.85rem; line-height: 1.6;
}
</style>
