<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * Soft delete for the admin's "Delete prompt": the turn goes to Deleted prompts,
 * can be restored there, and aiul:purge-deleted removes it for good after
 * config('aiul.trash_days').
 *
 * deletion_id ties together the rows deleted in one click (a prompt, its agent
 * steps and its answer) so they are restored or purged as one.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::table('ai_interactions', function (Blueprint $table) {
            $table->softDeletes();
            $table->foreignId('deleted_by')->nullable()->constrained('users')->nullOnDelete();
            $table->uuid('deletion_id')->nullable()->index();
        });

        // A session whose every row was deleted leaves the lists with them.
        Schema::table('ai_sessions', function (Blueprint $table) {
            $table->softDeletes();
        });
    }

    public function down(): void
    {
        Schema::table('ai_interactions', function (Blueprint $table) {
            $table->dropConstrainedForeignId('deleted_by');
            $table->dropColumn(['deletion_id', 'deleted_at']);
        });
        Schema::table('ai_sessions', function (Blueprint $table) {
            $table->dropSoftDeletes();
        });
    }
};
