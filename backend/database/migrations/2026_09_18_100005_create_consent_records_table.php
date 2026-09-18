<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * Consent, recorded in software rather than only in an employment contract.
 *
 * Also the audit log for raw-prompt access: viewing someone's actual prompt text
 * is a privileged action and every such view is written here. That rule does more
 * for adoption than any feature — it is the difference between coaching and
 * surveillance.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('consent_records', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('user_id')->constrained()->cascadeOnDelete();

            // "capture" — the person was told capture is active on their device.
            // "raw_view" — somebody opened raw prompt text.
            $table->string('kind');

            $table->string('policy_version')->nullable();
            $table->timestamp('granted_at')->nullable();
            $table->timestamp('revoked_at')->nullable();

            // For raw_view: who looked, at what, and why.
            $table->foreignId('actor_user_id')->nullable()->constrained('users')->nullOnDelete();
            $table->foreignId('ai_interaction_id')->nullable()->constrained()->nullOnDelete();
            $table->string('reason')->nullable();
            $table->string('ip')->nullable();

            $table->timestamps();

            $table->index(['tenant_id', 'user_id', 'kind']);
            $table->index(['tenant_id', 'kind', 'created_at']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('consent_records');
    }
};
