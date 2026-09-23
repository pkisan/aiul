<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * The account a person used inside the AI tool ("alex@example.com" in ChatGPT),
 * which is not always the person the device belongs to. Null means the tool did
 * not say; the dashboard then shows the device's person.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::table('ai_interactions', function (Blueprint $table) {
            $table->string('account')->nullable();
        });
    }

    public function down(): void
    {
        Schema::table('ai_interactions', function (Blueprint $table) {
            $table->dropColumn('account');
        });
    }
};
