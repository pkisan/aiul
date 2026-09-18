<?php

return [
    /*
     * How long a gap can fall between two interactions before they count as
     * separate sessions. This is the definition behind "AI time per task", so it
     * is a setting rather than a number buried in code — and the dashboard shows
     * it, because a metric nobody can explain is a metric nobody trusts.
     */
    'session_idle_minutes' => (int) env('AIUL_SESSION_IDLE_MINUTES', 30),

    /*
     * Where prompt and answer bodies are stored. 's3' is MinIO locally and real S3
     * in production.
     */
    'body_disk' => env('AIUL_BODY_DISK', 's3'),
];
