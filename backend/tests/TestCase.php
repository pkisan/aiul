<?php

namespace Tests;

use Illuminate\Contracts\Auth\Authenticatable;
use Illuminate\Foundation\Testing\TestCase as BaseTestCase;

abstract class TestCase extends BaseTestCase
{
    /**
     * Tests switch people inside one session, which a browser never does.
     * AuthenticateSession would read that as "password changed" and sign the
     * new person out, so forget the previous person's password hash first.
     */
    public function actingAs(Authenticatable $user, $guard = null)
    {
        $this->app['session']->forget('password_hash_web');

        return parent::actingAs($user, $guard);
    }
}
