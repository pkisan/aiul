<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * pm_task_keys: every key a task has ever had ("AAY-4", later "MOB-17"). The
 * linker reads branch keys through it, so a branch named after an old key
 * still finds the task after it moves project.
 *
 * pm_link_events: an append-only log of every change to a session's link, by
 * the linker (actor null) or by a person in the inbox. Rows are never updated.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('pm_task_keys', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('task_id')->constrained('pm_tasks')->cascadeOnDelete();
            $table->string('key', 16);
            $table->timestamps();
            $table->unique(['tenant_id', 'key']);
        });

        // Every existing task gets its current key.
        DB::statement("
            insert into pm_task_keys (tenant_id, task_id, key, created_at, updated_at)
            select t.tenant_id, t.id, p.key || '-' || t.number, now(), now()
            from pm_tasks t join pm_projects p on p.id = t.project_id
        ");

        Schema::create('pm_link_events', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('ai_session_id')->constrained()->cascadeOnDelete();
            $table->foreignId('from_task_id')->nullable()->constrained('pm_tasks')->nullOnDelete();
            $table->foreignId('to_task_id')->nullable()->constrained('pm_tasks')->nullOnDelete();
            // linked | suggested | unlinked (linker); confirmed | reassigned | not_work (a person)
            $table->string('action', 16);
            $table->string('method', 16)->nullable();
            $table->decimal('confidence', 3, 2)->nullable();
            $table->foreignId('actor_id')->nullable()->constrained('users')->nullOnDelete(); // null = the linker
            $table->timestamp('occurred_at');
            $table->index(['ai_session_id', 'occurred_at']);
            $table->index(['tenant_id', 'occurred_at']);
        });

        // Start the log from today's state: one event per existing link.
        DB::statement("
            insert into pm_link_events (tenant_id, ai_session_id, from_task_id, to_task_id, action, method, confidence, actor_id, occurred_at)
            select tenant_id, ai_session_id, null, task_id,
                   case when confirmed_by is not null and task_id is null then 'not_work'
                        when confirmed_by is not null then 'confirmed'
                        when confidence >= 0.8 then 'linked'
                        else 'suggested' end,
                   method, confidence, confirmed_by, coalesce(confirmed_at, updated_at)
            from pm_task_ai_links
        ");
    }

    public function down(): void
    {
        Schema::dropIfExists('pm_link_events');
        Schema::dropIfExists('pm_task_keys');
    }
};
