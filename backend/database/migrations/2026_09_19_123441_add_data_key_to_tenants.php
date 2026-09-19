<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * Per-tenant encryption keys (D9).
 *
 * data_key holds one customer's data key, itself encrypted — never the raw bytes.
 * Locally the wrapping is done with the application key; in production it is a KMS
 * call, and only the two wrap/unwrap methods in BodyStore change.
 *
 * Null means no key has been needed yet. It is created on first write, so nothing
 * has to be seeded and an existing tenant needs no migration of its own.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::table('tenants', function (Blueprint $table) {
            $table->text('data_key')->nullable()->after('retention_days');
        });
    }

    public function down(): void
    {
        Schema::table('tenants', function (Blueprint $table) {
            $table->dropColumn('data_key');
        });
    }
};
