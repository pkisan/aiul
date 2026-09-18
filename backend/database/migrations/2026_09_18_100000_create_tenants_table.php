<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * A tenant is one customer. Every other table in this module carries a tenant_id
 * and a global scope, so a forgotten `where` cannot leak one customer's prompts
 * into another's dashboard.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('tenants', function (Blueprint $table) {
            $table->id();
            $table->string('name');
            $table->string('slug')->unique();

            // Retention is per customer: purge raw prompt and answer bodies after
            // this many days, keeping the derived scores.
            $table->unsignedSmallInteger('retention_days')->default(90);

            $table->timestamps();
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('tenants');
    }
};
