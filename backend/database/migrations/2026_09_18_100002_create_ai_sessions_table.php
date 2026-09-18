<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * A session groups the interactions of one continuous piece of work: the same
 * device, tool and task, with no long gap between them. It is what "AI time per
 * task" is measured over.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('ai_sessions', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('device_id')->constrained()->cascadeOnDelete();
            $table->foreignId('user_id')->nullable()->constrained()->nullOnDelete();

            $table->string('tool')->nullable();     // claude-code, cursor, ...
            $table->string('task_id')->nullable();  // ABC-123, null means untagged
            $table->string('repo')->nullable();
            $table->string('branch')->nullable();

            $table->timestamp('started_at');
            $table->timestamp('ended_at')->nullable();

            $table->unsignedInteger('interaction_count')->default(0);

            $table->timestamps();

            // The dashboard's main questions: usage per task, and per device.
            $table->index(['tenant_id', 'task_id']);
            $table->index(['tenant_id', 'started_at']);
            $table->index(['device_id', 'tool', 'task_id', 'ended_at']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('ai_sessions');
    }
};
