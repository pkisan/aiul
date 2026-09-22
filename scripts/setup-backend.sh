#!/bin/bash
# setup-backend.sh — stand the backend up on a tester's Mac, from a clean checkout.
#
#   ./scripts/setup-backend.sh
#   AIUL_PORT=8090 ./scripts/setup-backend.sh     # if 8088 is taken
#
# It does the boring, forgettable half of a first run: data services, .env, key,
# migrations, object storage bucket, dev logins, compiled assets. It changes
# NOTHING outside this checkout and its Docker volumes — no system proxy, no
# certificates, no launchd jobs. That is the agent's half, and it comes second.
#
# Safe to run again: every step checks before it acts.

set -euo pipefail

cd "$(dirname "$0")/.."
BACKEND="$PWD/backend"
PORT="${AIUL_PORT:-8088}"

say() { printf '\n\033[1m%s\033[0m\n' "$1"; }
have() { command -v "$1" >/dev/null 2>&1; }

# ---- 1. what must already be here ------------------------------------------
say "Checking prerequisites"
missing=0
for tool in php composer node npm docker; do
  if have "$tool"; then
    printf '  %-10s %s\n' "$tool" "$("$tool" --version 2>&1 | head -1)"
  else
    printf '  %-10s MISSING\n' "$tool"
    missing=1
  fi
done
if [ "$missing" -ne 0 ]; then
  echo
  echo "Install what is missing first. On a Mac with Homebrew:"
  echo "  brew install php composer node"
  echo "  brew install --cask docker    # then open Docker Desktop once"
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker is installed but not running. Open Docker Desktop, then run this again."
  exit 1
fi

# ---- 2. data services -------------------------------------------------------
say "Starting Postgres, Redis and MinIO"
docker compose up -d
printf '  waiting for them to report healthy'
for _ in $(seq 1 60); do
  unhealthy="$(docker compose ps --format '{{.Health}}' | grep -cv healthy || true)"
  [ "$unhealthy" -eq 0 ] && break
  printf '.'
  sleep 2
done
echo
docker compose ps --format '  {{.Name}}\t{{.Status}}'

# ---- 3. configuration -------------------------------------------------------
say "Configuring the application"
if [ ! -f "$BACKEND/.env" ]; then
  cp "$BACKEND/.env.example" "$BACKEND/.env"
  echo "  wrote backend/.env from .env.example"
else
  echo "  backend/.env exists, left alone"
fi

# The URL the agent will post events to. Written into .env so the value lives in
# one place, and read back out in step 7 for the instructions.
if grep -q '^APP_URL=' "$BACKEND/.env"; then
  sed -i '' "s#^APP_URL=.*#APP_URL=http://127.0.0.1:$PORT#" "$BACKEND/.env"
fi

(cd "$BACKEND" && composer install --no-interaction --quiet)
grep -q '^APP_KEY=base64:' "$BACKEND/.env" || (cd "$BACKEND" && php artisan key:generate --ansi)

# ---- 4. database ------------------------------------------------------------
say "Creating the database schema"
(cd "$BACKEND" && php artisan migrate --force)

# ---- 5. object storage ------------------------------------------------------
say "Creating the bucket prompts and answers are stored in"
BUCKET="$(grep '^AWS_BUCKET=' "$BACKEND/.env" | cut -d= -f2)"
docker compose exec -T minio mc alias set local http://127.0.0.1:9000 aiul aiul_dev_password >/dev/null 2>&1 || true
if docker compose exec -T minio mc ls "local/$BUCKET" >/dev/null 2>&1; then
  echo "  bucket $BUCKET already exists"
else
  docker compose exec -T minio mc mb "local/$BUCKET" >/dev/null && echo "  created bucket $BUCKET"
fi

# ---- 6. people and assets ---------------------------------------------------
say "Creating the development logins"
(cd "$BACKEND" && php artisan db:seed --class=DevUsersSeeder --force)

say "Building the dashboard"
(cd "$BACKEND" && npm install --silent && npm run build)

# ---- 7. what to do next -----------------------------------------------------
say "Backend ready"
cat <<NEXT
  Start it (two terminals, both stay open):

    cd backend && php artisan serve --host=127.0.0.1 --port=$PORT
    cd backend && php artisan queue:work          # scores prompts; without it scores stay blank

  Then open  http://127.0.0.1:$PORT/usage  and sign in as admin@example.com
  with the password printed above.

  The endpoint the agent posts to is:

    http://127.0.0.1:$PORT/api/aiul/events

  Next, put the agent on this machine:

    ./scripts/enroll-device.sh

NEXT
