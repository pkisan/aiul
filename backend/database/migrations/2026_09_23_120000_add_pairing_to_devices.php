<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

/**
 * Device pairing: `aiul login` asks for a short code, the person types it into
 * the web app after signing in, and the device is theirs from then on.
 */
return new class extends Migration
{
    public function up(): void
    {
        Schema::table('devices', function (Blueprint $table) {
            $table->string('pair_code', 16)->nullable()->unique();
            $table->timestamp('pair_code_expires_at')->nullable();
        });
    }

    public function down(): void
    {
        Schema::table('devices', function (Blueprint $table) {
            $table->dropColumn(['pair_code', 'pair_code_expires_at']);
        });
    }
};
