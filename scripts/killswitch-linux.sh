#!/bin/bash
# killswitch-linux.sh — undo EVERY system change the AI Usage Logger makes to an
# Ubuntu (GNOME) machine. scripts/killswitch.sh hands over to this file on Linux,
# so the command is the same on every OS:
#
#   sudo ./scripts/killswitch.sh          # undo everything
#   ./scripts/killswitch.sh --dry-run     # only print what it WOULD do
#
# Idempotent: running it twice, or on a machine that was never configured, is
# safe and changes nothing.
#
# It reverses, in this order:
#   1. systemd units       aiul.service and aiul-helper.service (stops the proxy)
#   2. GNOME proxy         every desktop user's proxy, ONLY where it points at us
#   3. /etc/environment    the marked block that sets variables at login
#   4. CA trust            the system store and each user's Chrome store (NSS)
#   5. leftovers           binary, config, dev marker, service account
#
# It leaves /var/db/aiul (the worker's CA copy and spool) in place; the last
# lines say how to delete it.

set -u # no -e: every step is attempted even if one fails

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

# Keep in sync with internal/platform/*_linux.go.
UNITS="aiul.service aiul-helper.service"
UNIT_DIR="/etc/systemd/system"
ENV_FILE="/etc/environment"
BLOCK_BEGIN="# >>> AIUL BEGIN >>>"
BLOCK_END="# <<< AIUL END <<<"
CA_FILE="/usr/local/share/ca-certificates/aiul-dev-root.crt"
NSS_NICK="AIUL Dev Root"
SERVICE_USER="_aiul"
STATE_DIR="/var/db/aiul"
PROXY_PORT="8899"

say()  { printf '%s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }

run() {
  local desc="$1"; shift
  say "   $desc"
  say "   \$ $*"
  if [ "$DRY_RUN" -eq 1 ]; then
    say "   (dry run, not executed)"
  else
    "$@" >/dev/null 2>&1
  fi
}

if [ "$DRY_RUN" -eq 0 ] && [ "$(id -u)" -ne 0 ]; then
  say "This script needs root to change system settings."
  say "Re-run it as:  sudo $0"
  exit 1
fi

# Desktop users: a real login shell and a home directory, uid 1000 and up.
desktop_users() {
  getent passwd | awk -F: '$3 >= 1000 && $3 < 60000 && $6 ~ /^\/home\// {print $1":"$3":"$6}'
}

# as_user <name> <uid> <command...> — run as that person, on their session bus if
# they are logged in, else on a private one (dbus-run-session), so their dconf
# settings are changed either way.
as_user() {
  local name="$1" uid="$2"; shift 2
  if [ -S "/run/user/$uid/bus" ]; then
    runuser -u "$name" -- env DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$uid/bus" "$@"
  else
    runuser -u "$name" -- dbus-run-session -- "$@"
  fi
}

say "AI Usage Logger kill switch (Linux)"
[ "$DRY_RUN" -eq 1 ] && say "DRY RUN — nothing will be changed."

# ---------------------------------------------------------------------------
step "1/5  systemd units"
# systemd starts services at boot. stop ends the process now, disable stops it
# starting again. The worker first: it asks the helper to remove the proxy.
for unit in $UNITS; do
  if [ -f "$UNIT_DIR/$unit" ]; then
    run "stop and disable $unit" systemctl disable --now "$unit"
    run "remove $UNIT_DIR/$unit" rm -f "$UNIT_DIR/$unit"
  else
    say "   not present: $UNIT_DIR/$unit"
  fi
done
[ "$DRY_RUN" -eq 0 ] && systemctl daemon-reload >/dev/null 2>&1

# ---------------------------------------------------------------------------
step "2/5  GNOME proxy (per desktop user)"
# GNOME keeps the proxy per user (Settings > Network > Proxy). Chrome follows it.
# Only a setting that points at our port is reset; a proxy the person set up
# themselves is left alone.
if ! command -v gsettings >/dev/null 2>&1; then
  say "   gsettings not found (no GNOME); nothing to reset"
else
  while IFS=: read -r name uid home; do
    [ -z "$name" ] && continue
    mode=$(as_user "$name" "$uid" gsettings get org.gnome.system.proxy mode 2>/dev/null)
    port=$(as_user "$name" "$uid" gsettings get org.gnome.system.proxy.https port 2>/dev/null)
    if [ "$mode" = "'manual'" ] && [ "$port" = "$PROXY_PORT" ]; then
      say "   $name: proxy points at us"
      if [ "$DRY_RUN" -eq 1 ]; then
        say "   \$ gsettings set org.gnome.system.proxy mode none   (as $name)"
      else
        as_user "$name" "$uid" gsettings set org.gnome.system.proxy mode none >/dev/null 2>&1
        say "   reset to none"
      fi
    else
      say "   $name: not ours (mode=${mode:-unset})"
    fi
  done < <(desktop_users)
fi

# ---------------------------------------------------------------------------
step "3/5  $ENV_FILE block"
# pam_env reads /etc/environment at every login. Only our marked block goes.
if grep -qF "$BLOCK_BEGIN" "$ENV_FILE" 2>/dev/null; then
  if [ "$DRY_RUN" -eq 1 ]; then
    say "   \$ sed -i '/AIUL BEGIN/,/AIUL END/d' $ENV_FILE"
  else
    cp "$ENV_FILE" "${ENV_FILE}.aiul-backup.$(date +%Y%m%d%H%M%S)"
    sed -i "\|${BLOCK_BEGIN}|,\|${BLOCK_END}|d" "$ENV_FILE"
    say "   removed (backup kept alongside the file)"
  fi
else
  say "   no AIUL block in $ENV_FILE"
fi
say "   NOTE: terminals and apps started before this keep the variables until you log out and in."

# ---------------------------------------------------------------------------
step "4/5  CA trust"
if [ -f "$CA_FILE" ]; then
  run "remove $CA_FILE" rm -f "$CA_FILE"
  run "rebuild the system trust store" update-ca-certificates --fresh
else
  say "   not in the system store: $CA_FILE"
fi
# Chrome on Linux reads each user's own NSS database, not the system store.
if command -v certutil >/dev/null 2>&1; then
  while IFS=: read -r name uid home; do
    [ -z "$name" ] && continue
    db="sql:$home/.pki/nssdb"
    [ -d "$home/.pki/nssdb" ] || continue
    if runuser -u "$name" -- certutil -d "$db" -L -n "$NSS_NICK" >/dev/null 2>&1; then
      if [ "$DRY_RUN" -eq 1 ]; then
        say "   \$ certutil -d $db -D -n '$NSS_NICK'   (as $name)"
      else
        # One entry per install; loop until none remain.
        for _ in 1 2 3 4 5; do
          runuser -u "$name" -- certutil -d "$db" -D -n "$NSS_NICK" >/dev/null 2>&1 || break
        done
        say "   removed from $name's Chrome store"
      fi
    else
      say "   not in $name's Chrome store"
    fi
  done < <(desktop_users)
else
  say "   certutil not installed; no Chrome store to clean"
fi

# ---------------------------------------------------------------------------
step "5/5  leftovers"
for path in /usr/local/share/aiul /usr/local/bin/aiul /etc/aiul /etc/aiul-dev-unmanaged; do
  if [ -e "$path" ]; then
    run "remove $path" rm -rf "$path"
  else
    say "   not present: $path"
  fi
done
if getent passwd "$SERVICE_USER" >/dev/null; then
  run "remove the $SERVICE_USER service account" userdel "$SERVICE_USER"
else
  say "   no $SERVICE_USER service account"
fi
if [ -d "$STATE_DIR" ]; then
  say "   NOTE: $STATE_DIR still holds the worker's CA copy and spooled events."
  say "   Remove it by hand if you want it gone: sudo rm -rf $STATE_DIR"
fi

say ""
if [ "$DRY_RUN" -eq 1 ]; then
  say "Dry run finished. Nothing was changed."
else
  say "Kill switch finished. This machine no longer routes traffic through aiul."
  say "Log out and back in so every app drops the old variables."
fi
say ""
say "Verify by hand:"
say "  gsettings get org.gnome.system.proxy mode        # 'none'"
say "  grep -c AIUL /etc/environment                     # 0"
say "  systemctl status aiul                             # could not be found"
say "  ls /usr/local/bin/aiul                            # no such file"
say "  curl -sI https://example.com >/dev/null && echo 'internet works'"
exit 0
