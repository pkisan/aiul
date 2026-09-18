<?php

namespace App\Services;

/**
 * A heuristic quality score for a prompt, across six dimensions.
 *
 * What this is NOT: a judgement of the person. It is a rough, explainable signal
 * meant for coaching — "your prompts score low on constraints" is useful; "you are
 * a 42" is not. Every dimension returns a reason in plain English, and the
 * dashboard shows them, because a score nobody can question is a score nobody
 * trusts.
 *
 * It is deliberately heuristic rather than a model call: it must be cheap, it must
 * be stable over time, and it must not send anyone's prompt to a third party to be
 * judged.
 */
class PromptScorer
{
    /**
     * Bump this whenever the rules below change. Stored with every score, so old
     * and new scores are never silently compared.
     */
    public const RUBRIC_VERSION = 1;

    public function score(string $prompt): array
    {
        $prompt = trim($prompt);

        if ($prompt === '') {
            return [
                'version' => self::RUBRIC_VERSION,
                'score' => 0,
                'dimensions' => [],
                'reasons' => ['The prompt was empty or could not be read.'],
            ];
        }

        $dimensions = [
            'clear_goal' => $this->clearGoal($prompt),
            'context_given' => $this->contextGiven($prompt),
            'constraints_stated' => $this->constraintsStated($prompt),
            'expected_output' => $this->expectedOutput($prompt),
            'examples' => $this->examples($prompt),
            'focus' => $this->focus($prompt),
        ];

        // Equal weight: pretending we know the right weighting would be false
        // precision. Six dimensions, 0-100 each.
        $score = (int) round(array_sum(array_column($dimensions, 'score')) / count($dimensions));

        return [
            'version' => self::RUBRIC_VERSION,
            'score' => $score,
            'dimensions' => $dimensions,
            'reasons' => array_values(array_filter(array_column($dimensions, 'reason'))),
        ];
    }

    /** Is there an actual instruction, or just a topic? */
    private function clearGoal(string $prompt): array
    {
        $verbs = ['write', 'fix', 'explain', 'refactor', 'add', 'remove', 'create', 'build',
            'debug', 'review', 'implement', 'convert', 'migrate', 'optimise', 'optimize',
            'test', 'summarise', 'summarize', 'compare', 'find', 'update', 'translate'];

        $hasVerb = $this->containsAny($prompt, $verbs);
        $hasQuestion = str_contains($prompt, '?');

        if ($hasVerb) {
            return $this->dim(100, 'States what to do with a clear action.');
        }
        if ($hasQuestion) {
            return $this->dim(70, 'Asks a question, though no explicit action is named.');
        }

        return $this->dim(30, 'No clear action or question — it reads as a topic rather than a request.');
    }

    /** Is there enough around the request to act on it? */
    private function contextGiven(string $prompt): array
    {
        $signals = 0;

        if (preg_match('/```|\n {4}\S/', $prompt)) {
            $signals++; // a code block
        }
        if (preg_match('/\b[\w\/.-]+\.(php|go|js|ts|vue|py|sql|yml|yaml|json|md)\b/i', $prompt)) {
            $signals++; // a file is named
        }
        if (preg_match('/\b(error|exception|stack trace|fails? with|returns?|expected|actual)\b/i', $prompt)) {
            $signals++; // observed behaviour
        }
        if (mb_strlen($prompt) > 200) {
            $signals++;
        }

        return match (true) {
            $signals >= 3 => $this->dim(100, 'Gives code, files or observed behaviour to work from.'),
            $signals === 2 => $this->dim(75, 'Gives some context.'),
            $signals === 1 => $this->dim(50, 'Gives a little context; more would help.'),
            default => $this->dim(20, 'No code, files or error text — the model has to guess the situation.'),
        };
    }

    /** Are the boundaries stated: what to use, what to avoid? */
    private function constraintsStated(string $prompt): array
    {
        $words = ['must', 'should', 'do not', "don't", 'avoid', 'without', 'only', 'keep',
            'using', 'use ', 'instead of', 'no ', 'limit', 'at most', 'prefer'];

        $hits = $this->countAny($prompt, $words);

        return match (true) {
            $hits >= 3 => $this->dim(100, 'Sets clear boundaries on the answer.'),
            $hits >= 1 => $this->dim(65, 'States some constraints.'),
            default => $this->dim(25, 'No constraints given, so the answer may use anything it likes.'),
        };
    }

    /** Is the shape of a good answer described? */
    private function expectedOutput(string $prompt): array
    {
        $words = ['return', 'output', 'format', 'json', 'table', 'list', 'in one', 'bullet',
            'diff', 'patch', 'function', 'class', 'test', 'markdown', 'csv', 'sentences',
            'steps', 'summary', 'example'];

        $hits = $this->countAny($prompt, $words);

        return match (true) {
            $hits >= 2 => $this->dim(100, 'Says what the answer should look like.'),
            $hits === 1 => $this->dim(65, 'Hints at the expected form of the answer.'),
            default => $this->dim(30, 'Does not say what form the answer should take.'),
        };
    }

    /** Is there an example, which is usually worth a paragraph of description? */
    private function examples(string $prompt): array
    {
        if (preg_match('/\b(for example|e\.g\.|such as|like this|sample|here is an example)\b/i', $prompt)) {
            return $this->dim(100, 'Includes an example.');
        }
        if (preg_match('/```/', $prompt)) {
            return $this->dim(80, 'Includes a code block that serves as an example.');
        }

        return $this->dim(40, 'No example given. One example is often worth a paragraph of explanation.');
    }

    /** One thing asked well, or five things at once? */
    private function focus(string $prompt): array
    {
        $length = mb_strlen($prompt);
        $questions = substr_count($prompt, '?');
        $alsos = $this->countAny($prompt, ['also', 'and then', 'additionally', 'as well as', 'plus ']);

        if ($length > 6000) {
            return $this->dim(40, 'Very long — several requests in one usually get a worse answer than asking separately.');
        }
        if ($questions > 3 || $alsos >= 3) {
            return $this->dim(50, 'Asks for several different things at once.');
        }
        if ($length < 25) {
            return $this->dim(35, 'Very short — probably not enough for a useful answer.');
        }

        return $this->dim(100, 'Asks for one thing at a reasonable length.');
    }

    private function dim(int $score, string $reason): array
    {
        return ['score' => $score, 'reason' => $reason];
    }

    private function containsAny(string $haystack, array $needles): bool
    {
        return $this->countAny($haystack, $needles) > 0;
    }

    private function countAny(string $haystack, array $needles): int
    {
        $haystack = mb_strtolower($haystack);
        $count = 0;

        foreach ($needles as $needle) {
            if (str_contains($haystack, mb_strtolower($needle))) {
                $count++;
            }
        }

        return $count;
    }
}
