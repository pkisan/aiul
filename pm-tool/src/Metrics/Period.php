<?php

namespace Pm\Metrics;

use Illuminate\Support\Carbon;
use Pm\Models\Sprint;

/**
 * The time range an insight screen covers: the sprint running today, or the
 * last 7 or 30 days. ?period=sprint|7|30; the sprint when there is one.
 */
class Period
{
    public function __construct(
        public readonly string $key,
        public readonly string $label,
        public readonly Carbon $from,
        public readonly Carbon $to,
    ) {}

    public static function from(?string $key): self
    {
        // ponytail: one tenant-wide "current sprint" (the first found); per-project periods if projects run different sprints.
        $sprint = Sprint::whereDate('start_date', '<=', today())->whereDate('end_date', '>=', today())->orderBy('start_date')->first();
        $key = in_array($key, ['sprint', '7', '30'], true) ? $key : ($sprint ? 'sprint' : '7');

        if ($key === 'sprint' && $sprint) {
            return new self('sprint', $sprint->name, $sprint->start_date->copy()->startOfDay(), $sprint->end_date->copy()->endOfDay());
        }
        $days = $key === '30' ? 30 : 7;

        return new self((string) $days, "Last {$days} days", today()->subDays($days - 1), now()->endOfDay());
    }

    /** For the page's period picker. */
    public static function options(): array
    {
        $sprint = Sprint::whereDate('start_date', '<=', today())->whereDate('end_date', '>=', today())->orderBy('start_date')->first();

        return array_values(array_filter([
            $sprint ? ['key' => 'sprint', 'label' => "This sprint ({$sprint->name})"] : null,
            ['key' => '7', 'label' => 'Last 7 days'],
            ['key' => '30', 'label' => 'Last 30 days'],
        ]));
    }

    /** Each day in the period up to today, as Y-m-d (for sparklines and day bars). */
    public function days(): array
    {
        $days = [];
        for ($d = $this->from->copy()->startOfDay(); $d->lte(min($this->to, now())); $d->addDay()) {
            $days[] = $d->toDateString();
        }

        return $days;
    }

    public function toArray(): array
    {
        return ['key' => $this->key, 'label' => $this->label, 'from' => $this->from->toDateString(), 'to' => $this->to->toDateString()];
    }
}
