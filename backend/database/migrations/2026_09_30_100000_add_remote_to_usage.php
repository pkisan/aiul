<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * Where the checkout was cloned from, "github.com/acme/shop", with no
 * credentials (the agent strips them). `repo` is a local path and differs on
 * every machine; `remote` is the same for everyone on the project, so it is what
 * the PM tool links a project, and later its tasks, on. Null for work outside a
 * checkout and for events from agents older than v0.3.
 */
return new class extends Migration
{
    public function up(): void
    {
        foreach (['ai_interactions', 'ai_sessions'] as $name) {
            Schema::table($name, function (Blueprint $table) {
                $table->string('remote', 512)->nullable();
                $table->index(['tenant_id', 'remote']);
            });
        }
    }

    public function down(): void
    {
        foreach (['ai_interactions', 'ai_sessions'] as $name) {
            Schema::table($name, function (Blueprint $table) {
                $table->dropIndex(['tenant_id', 'remote']);
                $table->dropColumn('remote');
            });
        }
    }
};
