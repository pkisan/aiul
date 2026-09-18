<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * One row per managed machine running the agent. The device token authenticates
 * ingestion, and is stored only as a hash: a leaked database must not yield
 * working tokens.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('devices', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('user_id')->nullable()->constrained()->nullOnDelete();

            $table->string('hostname');
            $table->string('platform')->default('darwin');

            // sha256 of the token. Never the token itself.
            $table->string('token_hash', 64)->unique();

            $table->timestamp('last_seen_at')->nullable();
            $table->boolean('revoked')->default(false);
            $table->timestamps();

            $table->index(['tenant_id', 'hostname']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('devices');
    }
};
