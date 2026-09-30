<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * The PM module's tables. People, tenants and AI sessions are NOT copied here:
 * they are the logger's `users`, `tenants` and `ai_sessions` (D18, D19).
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('pm_projects', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->string('name');
            $table->string('key', 6); // AAY, the prefix of every task key
            $table->timestamps();
            $table->unique(['tenant_id', 'key']);
        });

        // Which code a project is: its git remotes (github.com/org/repo), the
        // same on every machine. A remote belongs to one project per tenant.
        Schema::create('pm_project_remotes', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('project_id')->constrained('pm_projects')->cascadeOnDelete();
            $table->string('remote', 512);
            $table->timestamps();
            $table->unique(['tenant_id', 'remote']);
        });

        Schema::create('pm_sprints', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('project_id')->constrained('pm_projects')->cascadeOnDelete();
            $table->string('name');
            $table->date('start_date');
            $table->date('end_date');
            $table->timestamps();
        });

        Schema::create('pm_tasks', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('project_id')->constrained('pm_projects')->cascadeOnDelete();
            $table->foreignId('sprint_id')->nullable()->constrained('pm_sprints')->nullOnDelete();
            $table->unsignedInteger('number'); // 42 in AAY-42
            $table->string('title');
            $table->text('description')->nullable();
            $table->string('status', 16)->default('backlog');
            $table->foreignId('assignee_id')->nullable()->constrained('users')->nullOnDelete();
            $table->timestamp('started_at')->nullable();   // first move to in_progress
            $table->timestamp('completed_at')->nullable(); // last move to done
            $table->timestamps();
            $table->unique(['project_id', 'number']);
            $table->index(['tenant_id', 'assignee_id', 'status']);
        });

        // Status and assignee changes, for cycle time and "stuck" signals.
        Schema::create('pm_task_events', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('task_id')->constrained('pm_tasks')->cascadeOnDelete();
            $table->foreignId('user_id')->nullable()->constrained()->nullOnDelete();
            $table->string('field', 16); // status | assignee
            $table->string('from')->nullable();
            $table->string('to')->nullable();
            $table->timestamp('occurred_at');
            $table->index(['task_id', 'occurred_at']);
        });

        // "Start working" on a task opens a period; stopping or switching closes
        // it. An AI session that starts inside a period links to that task.
        Schema::create('pm_work_periods', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('user_id')->constrained()->cascadeOnDelete();
            $table->foreignId('task_id')->constrained('pm_tasks')->cascadeOnDelete();
            $table->timestamp('started_at');
            $table->timestamp('ended_at')->nullable(); // null = the active task now
            $table->index(['user_id', 'started_at']);
        });
        // At most one active task per person, enforced by the database.
        DB::statement('create unique index pm_work_periods_one_open on pm_work_periods (user_id) where ended_at is null');

        // One row per AI session once the linker has looked at it. task_id null
        // with confirmed_by set means a person said "not task work".
        Schema::create('pm_task_ai_links', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('ai_session_id')->unique()->constrained()->cascadeOnDelete();
            $table->foreignId('task_id')->nullable()->constrained('pm_tasks')->cascadeOnDelete();
            $table->string('method', 16); // explicit | convention | time_window | manual
            $table->decimal('confidence', 3, 2);
            $table->foreignId('confirmed_by')->nullable()->constrained('users')->nullOnDelete();
            $table->timestamp('confirmed_at')->nullable();
            $table->timestamps();
            $table->index(['tenant_id', 'task_id']);
        });
    }

    public function down(): void
    {
        foreach (['pm_task_ai_links', 'pm_work_periods', 'pm_task_events', 'pm_tasks', 'pm_sprints', 'pm_project_remotes', 'pm_projects'] as $table) {
            Schema::dropIfExists($table);
        }
    }
};
