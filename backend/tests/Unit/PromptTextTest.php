<?php

namespace Tests\Unit;

use App\Services\PromptText;
use PHPUnit\Framework\TestCase;

class PromptTextTest extends TestCase
{
    public function test_claude_code_reminders_are_removed(): void
    {
        $raw = "<system-reminder>\nCLAUDE.md contents\n</system-reminder>\n<system-reminder>\ngit status\n</system-reminder>\n\nHi";

        $this->assertSame('Hi', PromptText::clean($raw));
    }

    public function test_slash_command_keeps_its_name(): void
    {
        $raw = '<local-command-caveat>Caveat</local-command-caveat><command-name>/clear</command-name>'
            .'<command-message>clear</command-message><command-args></command-args>'
            .'<local-command-stdout></local-command-stdout>';

        $this->assertSame('/clear', PromptText::clean($raw));
    }

    public function test_codex_context_with_attributes_is_removed(): void
    {
        $raw = "<permissions mode=\"x\">rules</permissions>\n<environment_context>\n<cwd>/x</cwd>\n</environment_context>\nHello";

        $this->assertSame('Hello', PromptText::clean($raw));
    }

    public function test_markup_the_person_typed_survives(): void
    {
        $raw = 'Why does <div class="x">a</div> not render? <system-reminderX>kept</system-reminderX>';

        $this->assertSame($raw, PromptText::clean($raw));
    }

    public function test_empty_stays_empty(): void
    {
        $this->assertNull(PromptText::clean(null));
        $this->assertSame('', PromptText::clean('<EPHEMERAL_MESSAGE>only context</EPHEMERAL_MESSAGE>'));
    }

    public function test_pasted_text_is_kept_without_its_wrapper(): void
    {
        $this->assertSame("log line\n\nwhy?", PromptText::clean("<pasted_content id=\"1\">\nlog line\n</pasted_content>\n\nwhy?"));
    }

    public function test_agent_written_prompts_are_recognised(): void
    {
        $this->assertTrue(PromptText::isToolGenerated('[SUGGESTION MODE: Suggest what the user might type'));
        $this->assertTrue(PromptText::isToolGenerated('The user stepped away and is coming back. Recap'));
        $this->assertFalse(PromptText::isToolGenerated('Hi'));
    }
}
