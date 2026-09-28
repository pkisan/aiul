<?php

namespace Database\Seeders;

use Illuminate\Database\Seeder;

class DatabaseSeeder extends Seeder
{
    /** A fresh installation gets exactly one account: the super admin. */
    public function run(): void
    {
        $this->call(SuperAdminSeeder::class);
    }
}
