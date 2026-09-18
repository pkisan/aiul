<?php

namespace App\Models\Concerns;

use App\Support\TenantContext;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/**
 * Row-level tenant isolation.
 *
 * Adding this trait to a model does two things:
 *   - every query is filtered to the current tenant, automatically
 *   - every new row gets the current tenant's id, automatically
 *
 * The point is that isolation does not depend on anyone remembering. A query
 * written without a `where tenant_id` is still safe.
 *
 * When no tenant is set — a console command, a test, a migration — nothing is
 * filtered. That is deliberate: the alternative is silently returning nothing and
 * leaving someone to debug an empty screen.
 */
trait BelongsToTenant
{
    public static function bootBelongsToTenant(): void
    {
        static::addGlobalScope('tenant', function (Builder $query) {
            $context = app(TenantContext::class);

            if ($context->has()) {
                $query->where($query->getModel()->getTable().'.tenant_id', $context->id());
            }
        });

        static::creating(function (Model $model) {
            if ($model->getAttribute('tenant_id') === null) {
                $model->setAttribute('tenant_id', app(TenantContext::class)->id());
            }
        });
    }

    public function tenant(): BelongsTo
    {
        return $this->belongsTo(\App\Models\Tenant::class);
    }
}
