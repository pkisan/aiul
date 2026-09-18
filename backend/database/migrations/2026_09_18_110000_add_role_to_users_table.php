<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * Roles decide what a person sees.
 *
 *   member  — their own data only
 *   manager — aggregates for their tenant: usage per task, per person, scores
 *   admin   — aggregates, plus the ability to open raw prompt text
 *
 * Seeing an actual prompt is a separate, deliberate step rather than something a
 * manager gets by default. That distinction is the difference between coaching
 * and surveillance, and it is why adoption survives.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::table('users', function (Blueprint $table) {
            $table->string('role')->default('member')->after('email');

            // Separate from the role on purpose: an admin still has to be granted
            // this explicitly, and it can be revoked without demoting anyone.
            $table->boolean('can_view_raw_prompts')->default(false)->after('role');
        });
    }

    public function down(): void
    {
        Schema::table('users', function (Blueprint $table) {
            $table->dropColumn(['role', 'can_view_raw_prompts']);
        });
    }
};
