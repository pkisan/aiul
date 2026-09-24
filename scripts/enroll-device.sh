#!/bin/bash
# enroll-device.sh — put the agent on this Mac or Ubuntu machine and point it at
# a backend.
#
#   ./scripts/enroll-device.sh                                  # backend on this machine
#   ./scripts/enroll-device.sh --user admin@example.com         # ...linked to that person
#   ./scripts/enroll-device.sh --endpoint https://aiul.example.com/api/aiul/events \
#                             --token aiul_xxx                  # backend elsewhere
#   ./scripts/enroll-device.sh --uninstall
#
# THIS ONE CHANGES THE MACHINE. It installs the agent, which sets the system
# proxy, trusts a locally generated CA, and runs two background services
# (launchd on macOS, systemd on Linux). Everything it does is undone by:
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
OS="$(uname -s)"   # Darwin or Linux

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

# ---- 1. the package (macOS) or the binary (Linux) ----------------------------
if [ "$OS" = "Linux" ]; then
  # Linux gets the bare binary: dist/aiul-linux-<cpu>, built on the Mac by
  # scripts/build.sh and copied here, or built here if Go is installed.
  case "$(uname -m)" in
    x86_64)        ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) echo "Unsupported CPU: $(uname -m)" >&2; exit 1 ;;
  esac
  BIN="$REPO/dist/aiul-linux-$ARCH"
  if [ ! -f "$BIN" ]; then
    if command -v go >/dev/null 2>&1; then
      say "Building the agent"
      mkdir -p "$REPO/dist"
      (cd "$REPO/agent" && CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X main.version=$(git -C "$REPO" describe --tags --always --dirty 2>/dev/null || echo dev)" \
        -o "$BIN" ./cmd/aiul)
    else
      echo "No $BIN and no Go to build it." >&2
      echo "On the Mac: ./scripts/build.sh, then copy dist/aiul-linux-$ARCH into dist/ here." >&2
      exit 1
    fi
  fi
  [ -x "$BIN" ] || chmod +x "$BIN"   # a copied file may have lost it
  echo "Agent: $BIN ($("$BIN" version))"
elif [ -z "$PKG" ]; then
  PKG="$(ls -t "$REPO"/dist/aiul-*.pkg 2>/dev/null | head -1 || true)"
fi
if [ -z "$PKG" ] || [ ! -f "$PKG" ]; then
  say "Building the package"
  "$REPO/scripts/build.sh" && "$REPO/scripts/package.sh"
  PKG="$(ls -t "$REPO"/dist/aiul-*.pkg | head -1)"
fi
[ "$OS" = "Linux" ] || echo "Package: $PKG"

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
      PROVISION=(php artisan aiul:provision-device "$(hostname -s)" --tenant=dev --platform="$(echo "$OS" | tr '[:upper:]' '[:lower:]')")
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
    if [ "$OS" = "Linux" ]; then
      echo "Docker is not reachable as $(whoami). Either start it (sudo systemctl start docker)" >&2
      echo "or let this user use it: sudo usermod -aG docker $(whoami), then log out and in." >&2
    else
      echo "Docker is not running. Open Docker Desktop, wait for it to start, then run this again." >&2
    fi
    exit 1
  fi
  echo "Could not obtain a device token. Is the backend running?" >&2
  echo "  docker compose -f compose.demo.yaml up -d --build" >&2
  exit 1
fi

# ---- 4. show everything before touching the machine -------------------------
MANAGED=0
# Linux has no MDM enrollment to check, so a Linux machine is always a test device.
if [ "$OS" = "Darwin" ] && profiles status -type enrollment 2>/dev/null | grep -q 'Enrolled via DEP: Yes\|MDM enrollment: Yes'; then
  MANAGED=1
fi

say "About to change this machine"
cat <<PLAN
  1. write /etc/aiul/agent.conf  (root-owned, 0600)
         AIUL_ENDPOINT=$ENDPOINT
         AIUL_DEVICE_TOKEN=aiul_… (hidden)
PLAN
[ "$MANAGED" -eq 0 ] && cat <<PLAN
  2. touch /etc/aiul-dev-unmanaged
         this machine is not MDM-enrolled, and the agent refuses to run on an
         unmanaged device unless this file exists. It is for testing only.
PLAN
NEED_NSS=0
if [ "$OS" = "Linux" ]; then
  command -v certutil >/dev/null 2>&1 || NEED_NSS=1
  [ "$NEED_NSS" -eq 1 ] && cat <<PLAN
  3a. sudo apt-get install -y libnss3-tools
         certutil, which tells Chrome to trust the CA (Chrome on Linux keeps
         its own list of trusted roots in ~/.pki/nssdb)
PLAN
  cat <<PLAN
  3. sudo install $(basename "$BIN") /usr/local/bin/aiul, then packaging/scripts/postinstall
         generates a CA for THIS machine and trusts it (system store and each
         desktop user's Chrome store), writes a marked block in
         /etc/environment, starts two systemd services (aiul-helper, aiul),
         and points each logged-in user's GNOME proxy at 127.0.0.1:8899

  Undo all of it, at any time, with:
         sudo $REPO/scripts/killswitch.sh

PLAN
else
cat <<PLAN
  3. sudo installer -pkg $(basename "$PKG") -target /
         installs /usr/local/bin/aiul, generates a CA for THIS machine and
         trusts it, writes a marked block in /etc/zshenv, sets the GUI session
         variables, starts two launchd jobs, and points the system proxy at
         127.0.0.1:8899

  Undo all of it, at any time, with:
         sudo $REPO/scripts/killswitch.sh

PLAN
fi

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
sudo chown root:0 /etc/aiul/agent.conf   # group wheel on macOS, root on Linux

if [ "$MANAGED" -eq 0 ]; then
  say "Marking this machine as an unmanaged test device"
  sudo touch /etc/aiul-dev-unmanaged
fi

say "Installing the agent"
if [ "$OS" = "Linux" ]; then
  if [ "$NEED_NSS" -eq 1 ]; then
    sudo apt-get install -y libnss3-tools
  fi
  sudo install -m 755 "$BIN" /usr/local/bin/aiul
  # The same script the macOS package runs after copying the binary.
  if ! sudo "$REPO/packaging/scripts/postinstall"; then
    echo "The install failed and rolled itself back. Why:" >&2
    sudo tail -20 /var/log/aiul-install.log >&2
    exit 1
  fi
else
  sudo installer -pkg "$PKG" -target /
fi

say "Checking it came up"
sudo /usr/local/bin/aiul status || true

if [ "$OS" = "Linux" ]; then
  cat <<LINUX

  On Linux, two more things before testing:
    - QUIT CHROME COMPLETELY and reopen it: it reads its trusted roots at start.
    - Log out and back in: /etc/environment (the variables for terminals and
      CLI tools) is read at login.
LINUX
fi
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
