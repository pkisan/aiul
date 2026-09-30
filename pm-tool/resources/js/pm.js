// Shared bits for the PM pages.

export const statusLabels = {
    backlog: 'Backlog',
    todo: 'To do',
    in_progress: 'In progress',
    in_review: 'In review',
    done: 'Done',
};

// "2026-09-14" as "14 Sep 2026"; dates from the server are plain dates or ISO.
export const day = (value) =>
    value ? new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '—';

export const usd = (n) => (n === null || n === undefined ? '—' : `$${Number(n).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`);
