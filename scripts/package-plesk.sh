#!/bin/bash
# package-plesk.sh — build the zip that is uploaded to Plesk for
# https://genailog.vardaam.site. See docs/DEPLOY-PLESK.md for the upload steps.
#
#   ./scripts/package-plesk.sh            # -> dist/genailog-<commit>.zip
#
# The zip is made from the last COMMIT (git archive), never from the working
# folder, so a local .env, node_modules, logs or half-finished edits cannot end
# up on the server. It contains production PHP dependencies (vendor/) and the
# built CSS/JS (public/build/), so the server needs no Composer or Node.
set -euo pipefail

cd "$(dirname "$0")/.."
REPO="$PWD"

if ! git diff --quiet HEAD -- backend pm-tool; then
  echo "backend/ or pm-tool/ has uncommitted changes; the zip is built from the last commit." >&2
  echo "Commit first, or run again to package the committed version anyway? [y/N]" >&2
  read -r answer
  [ "$answer" = "y" ] || exit 1
fi

SHA="$(git rev-parse --short HEAD)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "Exporting backend at $SHA"
git archive HEAD backend pm-tool | tar -x -C "$WORK"
cd "$WORK/backend"

# The PM module (pm-tool/) sits beside backend/ in the repo. The zip is the site
# root, so it goes inside it and composer is pointed at the new place.
cp -R ../pm-tool ./pm-tool
rm -rf pm-tool/tests pm-tool/docs
sed -i.bak 's#"\.\./pm-tool/#"pm-tool/#' composer.json && rm composer.json.bak

echo "Installing PHP dependencies (no dev packages)"
composer install --no-dev --optimize-autoloader --classmap-authoritative --no-interaction --no-progress --quiet

echo "Building CSS and JS"
npm ci --silent --no-audit --no-fund
VITE_APP_NAME="GenAI Log" npm run build --silent

# Only what runs on the server. Tests, Docker files and Node sources stay home.
rm -rf node_modules tests docker Dockerfile phpunit.xml .env.example \
       package.json package-lock.json vite.config.js tailwind.config.js \
       postcss.config.js jsconfig.json resources/js resources/css public/hot
# Empty folders Laravel writes to; git does not keep them.
mkdir -p storage/app/private storage/framework/{cache/data,sessions,views} storage/logs bootstrap/cache

mkdir -p "$REPO/dist"
OUT="$REPO/dist/genailog-$SHA.zip"
rm -f "$OUT"
zip -qr "$OUT" . -x '.git*'
echo "Built $OUT ($(du -h "$OUT" | cut -f1))"
