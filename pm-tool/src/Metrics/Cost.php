<?php

namespace Pm\Metrics;

use Illuminate\Database\Eloquent\Builder;

/**
 * Estimated cost of a set of AI interactions: tokens times the price of each
 * model (config/pm.php). Tokens are summed per model in SQL, so this is one
 * query whatever the number of rows.
 */
class Cost
{
    /** Price [input, output] per million tokens, or null when the model is unpriced. */
    public static function price(?string $model): ?array
    {
        $best = null;
        foreach (config('pm.prices') as $prefix => $price) {
            if ($model && str_starts_with($model, $prefix) && strlen($prefix) > strlen($best ?? '')) {
                $best = $prefix;
            }
        }

        return $best ? config('pm.prices')[$best] : null;
    }

    /**
     * @param  Builder  $interactions  a query on ai_interactions, already filtered
     * @return array{usd: float, tokens: int, priced_tokens: int}
     */
    public static function of(Builder $interactions): array
    {
        $rows = (clone $interactions)->reorder()
            ->selectRaw('model, sum(prompt_tokens) as input, sum(response_tokens) as output')
            ->groupBy('model')
            ->toBase()->get();

        $usd = 0.0;
        $tokens = $priced = 0;
        foreach ($rows as $r) {
            $tokens += $r->input + $r->output;
            if ($price = self::price($r->model)) {
                $usd += ($r->input * $price[0] + $r->output * $price[1]) / 1_000_000;
                $priced += $r->input + $r->output;
            }
        }

        return ['usd' => round($usd, 2), 'tokens' => (int) $tokens, 'priced_tokens' => (int) $priced];
    }
}
