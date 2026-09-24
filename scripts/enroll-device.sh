#!/bin/bash
# enroll-device.sh — put the agent on this Mac and point it at a backend.
#
#   ./scripts/enroll-device.sh                                  # backend on this machine
#   ./scripts/enroll-device.sh --user admin@example.com         # ...linked to that person
#   ./scripts/enroll-device.sh --endpoint https://aiul.example.com/api/aiul/events \
#                             --token aiul_xxx                  # backend elsewhere
#   ./scripts/enroll-device.sh --uninstall
#
# THIS ONE CHANGES THE MACHINE. It installs a package that sets the system proxy,
# trusts a locally generated CA, and runs two launchd jobs. Everything it does is
# undone by:
#
#   sudo ./scripts/killswitch.sh
#
# It asks before the part that needs root, and prints every command first.

set -euo pipefail

cd "$(dirname "$0")/.."
REPO="$PWD"
PORT="${AIUL_PORT:-8088}"
ENDPOINT=""
TOKEN=""
USER_EMAIL=""
PKG=""
UNINSTALL=0

while [ $# -gt 0 ]; do
  case "$1" in
    --endpoint) ENDPOINT="$2"; shift 2 ;;
    --token)    TOKEN="$2"; shift 2 ;;
    --user)     USER_EMAIL="$2"; shift 2 ;;
    --pkg)      PKG="$2"; shift 2 ;;
    --uninstall) UNINSTALL=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

say() { printf '\n\033[1m%s\033[0m\n' "$1"; }

if [ "$UNINSTALL" -eq 1 ]; then
  say "Removing the agent"
  echo "  sudo $REPO/scripts/killswitch.sh"
  sudo "$REPO/scripts/killswitch.sh"
  exit 0
fi

# ---- 1. the package ---------------------------------------------------------
if [ -z "$PKG" ]; then
  PKG="$(ls -t "$REPO"/dist/aiul-*.pkg 2>/dev/null | head -1 || true)"
fi
if [ -z "$PKG" ] || [ ! -f "$PKG" ]; then
  say "Building the package"
  "$REPO/scripts/build.sh" && "$REPO/scripts/package.sh"
  PKG="$(ls -t "$REPO"/dist/aiul-*.pkg | head -1)"
fi
echo "Package: $PKG"

# ---- 2. where events go -----------------------------------------------------
if [ -z "$ENDPOINT" ]; then
  ENDPOINT="http://127.0.0.1:$PORT/api/aiul/events"
fi

# ---- 3. a device token ------------------------------------------------------
# Provisioned against the backend in this checkout when none was given, which is
# the single-machine case. Pointing at someone else's backend means asking them
# for a token: this script cannot reach their database.
if [ -z "$TOKEN" ]; then
  case "$ENDPOINT" in
    http://127.0.0.1*|http://localhost*)
      say "Provisioning this device with the local backend"
      # The backend runs either in Docker (compose.demo.yaml) or on the host
      # (setup-backend.sh). The Docker one needs no PHP on this Mac.
      PROVISION=(php artisan aiul:provision-device "$(hostname -s)" --tenant=dev --platform=darwin)
      [ -n "$USER_EMAIL" ] && PROVISION+=(--user="$USER_EMAIL")
      if docker compose -f "$REPO/compose.demo.yaml" ps --status running -q app 2>/dev/null | grep -q .; then
        TOKEN="$(docker compose -f "$REPO/compose.demo.yaml" exec -T app "${PROVISION[@]}" 2>/dev/null \
                   | grep -oE 'aiul_[A-Za-z0-9_-]+' | head -1)"
      else
        TOKEN="$(cd "$REPO/backend" && "${PROVISION[@]}" 2>/dev/null | grep -oE 'aiul_[A-Za-z0-9_-]+' | head -1)"
      fi
      ;;
    *)
      echo "A remote endpoint needs a token from whoever runs that backend:" >&2
      echo "  php artisan aiul:provision-device <hostname> --tenant=<slug>" >&2
      echo "Then pass it here with --token." >&2
      exit 1
      ;;
  esac
fi

if [ -z "$TOKEN" ]; then
  if command -v docker >/dev/null 2>&1 && ! docker info >/dev/null 2>&1; then
    echo "Docker is not running. Open Docker Desktop, wait for it to start, then run this again." >&2
    exit 1
  fi
  echo "Could not obtain a device token. Is the backend running?" >&2
  echo "  docker compose -f compose.demo.yaml up -d --build" >&2
  exit 1
fi

# ---- 4. show everything before touching the machine -------------------------
MANAGED=0
if profiles status -type enrollment 2>/dev/null | grep -q 'Enrolled via DEP: Yes\|MDM enrollment: Yes'; then
  MANAGED=1
fi

say "About to change this Mac"
cat <<PLAN
  1. write /etc/aiul/agent.conf  (root-owned, 0600)
         AIUL_ENDPOINT=$ENDPOINT
         AIUL_DEVICE_TOKEN=aiul_… (hidden)
PLAN
[ "$MANAGED" -eq 0 ] && cat <<PLAN
  2. touch /etc/aiul-dev-unmanaged
         this Mac is not MDM-enrolled, and the agent refuses to run on an
         unmanaged device unless this file exists. It is for testing only.
PLAN
cat <<PLAN
  3. sudo installer -pkg $(basename "$PKG") -target /
         installs /usr/local/bin/aiul, generates a CA for THIS machine and
         trusts it, writes a marked block in /etc/zshenv, sets the GUI session
         variables, starts two launchd jobs, and points the system proxy at
         127.0.0.1:8899

  Undo all of it, at any time, with:
         sudo $REPO/scripts/killswitch.sh

PLAN

read -r -p "Proceed? [y/N] " answer
case "$answer" in
  y|Y|yes|YES) ;;
  *) echo "Nothing was changed."; exit 0 ;;
esac

# ---- 5. do it ---------------------------------------------------------------
say "Writing /etc/aiul/agent.conf"
sudo mkdir -p /etc/aiul
printf 'AIUL_ENDPOINT=%s\nAIUL_DEVICE_TOKEN=%s\n' "$ENDPOINT" "$TOKEN" | sudo tee /etc/aiul/agent.conf >/dev/null
sudo chmod 600 /etc/aiul/agent.conf
sudo chown root:wheel /etc/aiul/agent.conf

if [ "$MANAGED" -eq 0 ]; then
  say "Marking this Mac as an unmanaged test device"
  sudo touch /etc/aiul-dev-unmanaged
fi

say "Installing the agent"
sudo installer -pkg "$PKG" -target /

say "Checking it came up"
sudo /usr/local/bin/aiul status || true

cat <<DONE

  Events are only kept once this device belongs to a person who accepted the
  notice: sign in to the dashboard as that person once and accept it (or run
  \`aiul login\` if no --user was given).

  Send one prompt from Claude Code, Codex or claude.ai, then look at:

      $(dirname "$ENDPOINT" | sed 's#/api/aiul##')/usage

  If nothing appears, the log says why:

      tail -f /var/log/aiul/agent.err.log

  Remove everything:

      sudo $REPO/scripts/killswitch.sh

DONE
