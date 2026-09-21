// Shared formatting, so a duration or a timestamp never reads two different ways
// on two pages.

export const duration = (seconds) => {
    if (!seconds) return '—';
    if (seconds < 60) return `${seconds}s`;
    const h = Math.floor(seconds / 3600);
    const m = Math.round((seconds % 3600) / 60);
    return h ? `${h}h ${m}m` : `${m}m`;
};

export const when = (value) => (value ? new Date(value).toLocaleString() : '—');

export const clock = (value) =>
    value ? new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '—';

export const count = (n) => (n ?? 0).toLocaleString();
