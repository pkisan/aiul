<?php

return [
    /*
     * US dollars per million tokens, [input, output], by model id prefix. The
     * longest matching prefix wins, so "claude-sonnet-5-5" beats "claude-sonnet-5",
     * and dated ids ("claude-haiku-4-5-20251001") match their family.
     *
     * Claude prices: Anthropic API list prices, cached 2026-09-25. Other vendors
     * are left null on purpose: we have not checked their prices, and a made-up
     * number is worse than "unpriced". Fill them in to include them in cost.
     *
     * Cost is an ESTIMATE at API list price. Subscription chat apps (claude.ai,
     * chatgpt.com) are not billed per token, so for them it reads as "what this
     * would cost on the API".
     */
    'prices' => [
        'claude-fable-5' => [10.00, 50.00],
        'claude-opus-5-5' => [4.00, 20.00],
        'claude-opus-5' => [5.00, 25.00],
        'claude-opus-4-8' => [5.00, 25.00],
        'claude-opus-4-7' => [5.00, 25.00],
        'claude-opus-4-6' => [5.00, 25.00],
        'claude-sonnet-5-5' => [2.00, 10.00],
        'claude-sonnet-5' => [2.00, 10.00],
        'claude-sonnet-4-6' => [3.00, 15.00],
        'claude-haiku-4-5' => [1.00, 5.00],
        'gpt-5' => null,
        'gemini-3' => null,
    ],

    // Needs attention: this many prompts on one in-progress task within the
    // window, with no status change in that window, reads as "maybe stuck".
    'churn_prompts' => 10,
    'churn_hours' => 48,
];
