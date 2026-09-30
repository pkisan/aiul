<?php

namespace Pm\Models;

use App\Models\Concerns\BelongsToTenant;
use Illuminate\Database\Eloquent\Model;

/** A git remote (github.com/org/repo) that belongs to a project. */
class ProjectRemote extends Model
{
    use BelongsToTenant;

    protected $table = 'pm_project_remotes';

    protected $guarded = [];
}
