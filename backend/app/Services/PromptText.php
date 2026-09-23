<?php

namespace App\Services;

/**
 * The words a person actually typed, without what the tool wrapped around them.
 *
 * Agents send the model far more than the person wrote: Claude Code prepends
 * <system-reminder> blocks (CLAUDE.md, git status), Codex adds
 * <environment_context>, Antigravity an <EPHEMERAL_MESSAGE>. A prompt of "Hi"
 * arrived as 6,500 characters. Those blocks are the tool's, not the person's, so
 * they are removed before storage and again on display (rows stored before this
 * existed).
 *
 * Only whole blocks of the tags listed here are removed — never arbitrary markup,
 * which a person may well paste into a prompt.
 */
class PromptText
{
    /** Blocks dropped entirely, content and all. */
    private const WRAPPERS = [
        // Claude Code
        'system-reminder', 'local-command-caveat', 'local-command-stdout', 'local-command-stderr',
        'command-message', 'command-args', 'ide_opened_file', 'ide_selection', 'ide_diagnostics',
        // Codex
        'environment_context', 'user_instructions', 'INSTRUCTIONS', 'permissions', 'skills_instructions',
        'collaboration_mode', 'apps_instructions', 'plugins_instructions', 'recommended_plugins', 'model_switch',
        // Antigravity
        'EPHEMERAL_MESSAGE',
    ];

    /**
     * Openings of messages an agent writes as if the person had typed them. Claude
     * Code asks its model to guess the next prompt and to recap after a break, both
     * as ordinary user messages; 95 of one session's "prompts" were these.
     */
    private const TOOL_GENERATED = [
        '[SUGGESTION MODE:',
        'The user stepped away and is coming back.',
    ];

    /** Whether a cleaned prompt was written by the tool, not the person. */
    public static function isToolGenerated(?string $clean): bool
    {
        foreach (self::TOOL_GENERATED as $opening) {
            if (str_starts_with((string) $clean, $opening)) {
                return true;
            }
        }

        return false;
    }

    public static function clean(?string $text): ?string
    {
        if ($text === null || $text === '') {
            return $text;
        }

        // A slash command ("/clear") is sent as <command-name>/clear</command-name>:
        // the name IS what was typed, so keep it and drop the tags.
        $text = preg_replace('#<command-name>(.*?)</command-name>#s', '$1', $text);

        // Pasted text is the person's own; only the wrapper tag goes.
        $text = preg_replace('#<pasted_content(?:\s[^>]*)?>\n?(.*?)\n?</pasted_content>#s', '$1', $text);

        $names = implode('|', array_map(fn ($t) => preg_quote($t, '#'), self::WRAPPERS));

        // (?:\s[^>]*)? allows attributes (<permissions mode="...">) but not a
        // longer tag name that merely starts the same way.
        $text = preg_replace('#<('.$names.')(?:\s[^>]*)?>.*?</\1>#s', '', $text);

        return trim($text);
    }
}
