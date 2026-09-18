<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * One captured request and response.
 *
 * The prompt and answer text is NOT stored here: it goes to object storage,
 * encrypted, and this row keeps only the key. That keeps the hot table small and
 * means retention can delete the bodies while the metrics survive.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('ai_interactions', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('device_id')->constrained()->cascadeOnDelete();
            $table->foreignId('ai_session_id')->nullable()->constrained()->nullOnDelete();
            $table->foreignId('user_id')->nullable()->constrained()->nullOnDelete();

            // The id the agent generated. Unique per tenant, so a batch that is
            // sent twice after a retry cannot create duplicates.
            $table->string('event_id', 64);

            $table->string('host');
            $table->string('path')->nullable();
            $table->unsignedSmallInteger('status')->nullable();
            $table->string('tool')->nullable();
            $table->string('model')->nullable();
            $table->string('parser')->nullable();

            $table->string('task_id')->nullable();
            $table->string('repo')->nullable();
            $table->string('branch')->nullable();

            // Where the bodies live in object storage. Null means there were none
            // or they have been purged by retention.
            $table->string('prompt_object')->nullable();
            $table->string('answer_object')->nullable();

            // Kept in the row for search and for showing a preview without
            // fetching and decrypting the whole body.
            $table->unsignedInteger('prompt_chars')->default(0);
            $table->unsignedInteger('answer_chars')->default(0);

            $table->unsignedInteger('prompt_tokens')->default(0);
            $table->unsignedInteger('response_tokens')->default(0);
            $table->unsignedBigInteger('request_bytes')->default(0);
            $table->unsignedBigInteger('response_bytes')->default(0);
            $table->unsignedInteger('duration_ms')->default(0);

            $table->boolean('streamed')->default(false);

            // True when the wire format showed this was an agent's own follow-up
            // rather than something a person typed. Counting those as prompts
            // would make one question look like twenty.
            $table->boolean('automated')->default(false);

            // Which redaction rules fired, by name. Never a value.
            $table->jsonb('redacted')->nullable();
            $table->unsignedSmallInteger('redaction_rules_version')->default(0);
            $table->unsignedSmallInteger('allowlist_version')->default(0);

            $table->timestamp('occurred_at');
            $table->timestamps();

            $table->unique(['tenant_id', 'event_id']);
            $table->index(['tenant_id', 'task_id', 'occurred_at']);
            $table->index(['tenant_id', 'occurred_at']);
            $table->index(['tenant_id', 'tool']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('ai_interactions');
    }
};
