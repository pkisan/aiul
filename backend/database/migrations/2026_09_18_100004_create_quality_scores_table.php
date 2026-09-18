<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * A heuristic score for one prompt, with the reasoning kept alongside it.
 *
 * The rubric version matters: when the scoring changes, old scores must not be
 * silently compared against new ones, and a person asking "why did I get a 3?"
 * has to be able to see which rules were applied.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::create('quality_scores', function (Blueprint $table) {
            $table->id();
            $table->foreignId('tenant_id')->constrained()->cascadeOnDelete();
            $table->foreignId('ai_interaction_id')->constrained()->cascadeOnDelete();

            $table->unsignedSmallInteger('rubric_version');

            // 0-100 overall, and the per-dimension breakdown that produced it.
            $table->unsignedSmallInteger('score');
            $table->jsonb('dimensions');

            // Plain-English reasons, shown to the person whose prompt it was.
            $table->jsonb('reasons')->nullable();

            $table->timestamps();

            $table->unique(['ai_interaction_id', 'rubric_version']);
            $table->index(['tenant_id', 'score']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('quality_scores');
    }
};
