<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * Who caused a request: "human", "agent" or "utility".
 *
 * The boolean `automated` could not carry this. One message a person types
 * produces one human request, a dozen agent ones as the tool works through its
 * tools, and a handful of utility calls the tool makes for itself — and the
 * dashboard has to show a person their own turns, not forty rows.
 *
 * Existing rows keep `automated` and get no kind: their flag was computed by the
 * old rule, which marked a person's own prompt automated as soon as the
 * conversation had used a single tool, so guessing a kind from it would write
 * down something known to be wrong.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::table('ai_interactions', function (Blueprint $table) {
            $table->string('kind', 16)->nullable()->after('automated');
            $table->index(['tenant_id', 'kind']);
        });
    }

    public function down(): void
    {
        Schema::table('ai_interactions', function (Blueprint $table) {
            $table->dropIndex(['tenant_id', 'kind']);
            $table->dropColumn('kind');
        });
    }
};
