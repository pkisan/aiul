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

// "5m ago", "3h ago", "2d ago": for lists where the exact time matters less
// than which is newest.
export const ago = (value) => {
    if (!value) return '—';
    const s = Math.max(0, (Date.now() - new Date(value).getTime()) / 1000);
    if (s < 60) return 'just now';
    if (s < 3600) return `${Math.floor(s / 60)}m ago`;
    if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
    return `${Math.floor(s / 86400)}d ago`;
};

// The name a person knows a tool by. Unknown tools keep their own id.
const toolNames = {
    'chatgpt-web': 'ChatGPT',
    'claude-web': 'Claude',
    'claude-code': 'Claude Code',
    codex: 'Codex',
    cursor: 'Cursor',
    antigravity: 'Antigravity',
    'copilot-web': 'Copilot',
    'copilot-vscode': 'Copilot in VS Code',
    githubcopilotchat: 'Copilot in VS Code',
    // Claude Code identifies itself to api.anthropic.com as "cli".
    cli: 'Claude Code',
};
export const toolName = (id) => toolNames[id] ?? id ?? 'AI tool';

// "Punit Kisan" -> "PK", for avatars.
export const initials = (name) =>
    (name ?? '?')
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map((w) => w[0].toUpperCase())
        .join('') || '?';
