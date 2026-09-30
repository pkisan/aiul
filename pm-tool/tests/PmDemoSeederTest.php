<?php

namespace Pm\Tests;

use App\Models\Tenant;
use App\Support\TenantContext;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Storage;
use Pm\Database\Seeders\PmDemoSeeder;
use Pm\Models\Task;
use Pm\Models\TaskAiLink;
use Tests\TestCase;

class PmDemoSeederTest extends TestCase
{
    use RefreshDatabase;

    public function test_it_builds_the_sketch_trail_for_aay_1(): void
    {
        Storage::fake('s3');
        $this->seed(PmDemoSeeder::class);
        app(TenantContext::class)->set(Tenant::where('slug', 'aayatti')->value('id'));

        $task = Task::with('project')->where('number', 1)->firstOrFail();
        $this->assertSame('AAY-1', $task->key);
        $this->assertSame('Parit', $task->assignee->name);

        $link = $task->aiLinks()->sole();
        $this->assertSame(TaskAiLink::EXPLICIT, $link->method);
        $this->assertSame(1.0, $link->confidence);

        $session = $link->session;
        $this->assertSame('2026-09-14 10:34', $session->started_at->tz('Asia/Kolkata')->format('Y-m-d H:i'));
        $this->assertSame('14:15', $session->ended_at->tz('Asia/Kolkata')->format('H:i'));

        $prompts = $session->interactions()->where('kind', 'human')->orderBy('occurred_at')->get();
        $this->assertSame(['10:34', '11:48', '13:02', '14:15'], $prompts->map(fn ($i) => $i->occurred_at->tz('Asia/Kolkata')->format('H:i'))->all());
        $this->assertNotNull($prompts[0]->prompt_object);

        // Every board column has a task, and the seeder is safe to rerun.
        $this->assertEqualsCanonicalizing(Task::STATUSES, Task::distinct()->pluck('status')->all());
        $this->seed(PmDemoSeeder::class);
        $this->assertSame(1, Tenant::where('slug', 'aayatti')->count());
    }
}
