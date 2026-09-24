#!/bin/sh
# Container entry point for the demo backend (compose.demo.yaml).
#
#   serve   first-run setup, then the web app on port 8088
#   worker  the queue worker that scores prompts (waits for serve's setup)
#
# Everything first-run lives in /app/storage, a Docker volume, so it survives the
# container being rebuilt: the app key and the "already seeded" marker.
# Configuration otherwise comes from environment variables set in
# compose.demo.yaml; Laravel prefers those over any .env file.
set -eu
cd /app

STATE=storage/app/aiul-demo
mkdir -p "$STATE" storage/framework/cache/data storage/framework/sessions \
  storage/framework/views storage/logs

# The app key encrypts sessions and stored prompt bodies. Made once; losing it
# makes every stored body unreadable, which is why it lives in the volume.
# Written to .env too, so `docker compose exec app php artisan ...` sees it.
if [ ! -s "$STATE/app-key" ]; then
  php artisan key:generate --show --no-ansi > "$STATE/app-key"
fi
printf 'APP_KEY=%s\n' "$(cat "$STATE/app-key")" > .env

case "${1:-serve}" in
  serve)
    # Postgres reports healthy before compose starts us, but be patient anyway.
    tries=0
    until php artisan migrate --force --no-interaction; do
      tries=$((tries + 1))
      [ "$tries" -ge 15 ] && exit 1
      sleep 2
    done

    # Seeding rotates the dev passwords, so it runs once, not on every start.
    # The password is printed once, to this container's log:
    #   docker compose -f compose.demo.yaml logs app | grep -A1 Password
    if [ ! -f "$STATE/seeded" ]; then
      php artisan db:seed --class=DevUsersSeeder --force --no-ansi
      touch "$STATE/seeded"
    fi

    exec php artisan serve --host=0.0.0.0 --port=8088 --no-reload
    ;;
  worker)
    exec php artisan queue:work --tries=3
    ;;
  *)
    exec "$@"
    ;;
esac
