#!/bin/bash
# killswitch.sh — undo EVERY system change the AI Usage Logger makes to this Mac.
#
# Run this any time something feels wrong. It is idempotent: running it twice, or
# running it when nothing was ever configured, is safe and changes nothing.
#
#   sudo ./scripts/killswitch.sh          # undo everything
#   ./scripts/killswitch.sh --dry-run     # only print what it WOULD do
#
# It reverses, in this order:
#   1. launchd jobs        our LaunchDaemon and LaunchAgent (stops the proxy)
#   2. system proxy        HTTP/HTTPS proxy on every network service
#   3. launchctl env vars  the GUI-wide variables set at login
#   4. /etc/zshenv block   the marked block that sets variables for terminals
#   5. keychain trust      our dev root CA removed from the System keychain
#   6. leftovers           the installed binary, the service account, the token
#
# It deliberately does NOT delete ~/Library/Application Support/AIUL/dev-ca/, so a
# CA can be re-trusted later instead of regenerated. Delete that folder by hand if
# you want the key gone.

set -u # unset variable is an error; no -e, we want every step attempted

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

# Markers and paths this project owns. Keep in sync with internal/platform/darwin.
ZSHENV_FILE="/etc/zshenv"
BLOCK_BEGIN="# >>> AIUL BEGIN >>>"
BLOCK_END="# <<< AIUL END <<<"
DAEMON_PLIST="/Library/LaunchDaemons/com.aiul.agent.plist"
HELPER_PLIST="/Library/LaunchDaemons/com.aiul.helper.plist"
SERVICE_USER="_aiul"
STATE_DIR="/var/db/aiul"
AGENT_PLIST="/Library/LaunchAgents/com.aiul.session.plist"
CA_COMMON_NAME_PREFIX="AIUL Dev Root"
ENV_VARS="HTTPS_PROXY HTTP_PROXY NO_PROXY NODE_EXTRA_CA_CERTS NODE_USE_SYSTEM_CA SSL_CERT_FILE CODEX_CA_CERTIFICATE CLAUDE_CODE_CERT_STORE REQUESTS_CA_BUNDLE"

changed=0

say()  { printf '%s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }

# run <description> <command...> — prints the command, then runs it unless --dry-run.
run() {
  local desc="$1"; shift
  say "   $desc"
  say "   \$ $*"
  if [ "$DRY_RUN" -eq 1 ]; then
    say "   (dry run, not executed)"
  else
    "$@" >/dev/null 2>&1
    changed=1
  fi
}

need_root() {
  if [ "$DRY_RUN" -eq 0 ] && [ "$(id -u)" -ne 0 ]; then
    say "This script needs root to change system settings."
    say "Re-run it as:  sudo $0"
    exit 1
  fi
}

need_root

say "AI Usage Logger kill switch"
[ "$DRY_RUN" -eq 1 ] && say "DRY RUN — nothing will be changed."

# ---------------------------------------------------------------------------
step "1/6  launchd jobs"
# launchd is macOS's service manager. Unloading a job stops the process and
# prevents it starting again at boot or login.
# The worker first, then the helper: the worker asks the helper to remove the
# system proxy on its way out.
for plist in "$DAEMON_PLIST" "$HELPER_PLIST" "$AGENT_PLIST"; do
  if [ -f "$plist" ]; then
    run "unload $plist" launchctl unload -w "$plist"
    run "remove $plist" rm -f "$plist"
  else
    say "   not present: $plist"
  fi
done
# bootout also catches a job that was loaded without a plist on disk.
if [ "$DRY_RUN" -eq 0 ]; then
  launchctl bootout system/com.aiul.agent >/dev/null 2>&1
  launchctl bootout system/com.aiul.helper >/dev/null 2>&1
  launchctl bootout "gui/$(stat -f %u /dev/console)/com.aiul.session" >/dev/null 2>&1
fi

# ---------------------------------------------------------------------------
step "2/6  system proxy on every network service"
# networksetup is the macOS command-line tool for network settings. A "network
# service" is one entry in System Settings > Network: Wi-Fi, Ethernet, a VPN.
# We turn the proxy off on all of them, because the agent may have set several.
services=$(networksetup -listallnetworkservices 2>/dev/null | tail -n +2 | sed 's/^\*//')
if [ -z "$services" ]; then
  say "   could not list network services (is networksetup available?)"
else
  while IFS= read -r svc; do
    [ -z "$svc" ] && continue
    web=$(networksetup -getwebproxy "$svc" 2>/dev/null | awk '/^Enabled:/{print $2}')
    sec=$(networksetup -getsecurewebproxy "$svc" 2>/dev/null | awk '/^Enabled:/{print $2}')
    if [ "$web" = "Yes" ] || [ "$sec" = "Yes" ]; then
      run "disable HTTP proxy on '$svc'"  networksetup -setwebproxystate       "$svc" off
      run "disable HTTPS proxy on '$svc'" networksetup -setsecurewebproxystate "$svc" off
    else
      say "   already off: $svc"
    fi
  done <<< "$services"
fi

# ---------------------------------------------------------------------------
step "3/6  launchctl environment variables (GUI apps)"
# launchctl setenv sets a variable for GUI applications launched afterwards.
# unsetenv removes it. Apps already running keep their copy until restarted.
console_uid=$(stat -f %u /dev/console 2>/dev/null || echo 0)
for var in $ENV_VARS; do
  if [ "$DRY_RUN" -eq 1 ]; then
    say "   \$ launchctl unsetenv $var"
  else
    launchctl unsetenv "$var" >/dev/null 2>&1
    # also clear it in the logged-in user's GUI session
    [ "$console_uid" -ne 0 ] && launchctl asuser "$console_uid" launchctl unsetenv "$var" >/dev/null 2>&1
  fi
done
say "   cleared: $ENV_VARS"
say "   NOTE: already-running apps keep their old copy until you quit and reopen them."

# ---------------------------------------------------------------------------
step "4/6  /etc/zshenv block (terminals)"
# /etc/zshenv is read by every zsh shell, including non-interactive ones. We only
# ever write between our two markers, so we can remove exactly our block and
# leave anything else in the file untouched.
if [ -f "$ZSHENV_FILE" ] && grep -qF "$BLOCK_BEGIN" "$ZSHENV_FILE" 2>/dev/null; then
  say "   found our block in $ZSHENV_FILE"
  if [ "$DRY_RUN" -eq 1 ]; then
    say "   \$ sed -i '' '/AIUL BEGIN/,/AIUL END/d' $ZSHENV_FILE"
  else
    cp "$ZSHENV_FILE" "${ZSHENV_FILE}.aiul-backup.$(date +%Y%m%d%H%M%S)"
    sed -i '' "\|${BLOCK_BEGIN}|,\|${BLOCK_END}|d" "$ZSHENV_FILE"
    changed=1
    say "   removed (backup kept alongside the file)"
    # If the file is now empty apart from whitespace, remove it — we created it.
    if [ ! -s "$ZSHENV_FILE" ] || [ -z "$(tr -d '[:space:]' < "$ZSHENV_FILE")" ]; then
      rm -f "$ZSHENV_FILE"
      say "   $ZSHENV_FILE was empty afterwards, removed"
    fi
  fi
else
  say "   no AIUL block in $ZSHENV_FILE"
fi

# ---------------------------------------------------------------------------
step "5/6  dev root CA trust in the System keychain"
# A certificate in the System keychain marked as trusted is believed by Safari,
# Chrome, curl and most macOS software. Removing it makes our minted certificates
# fail again, which is exactly what we want when shutting everything down.
KEYCHAIN="/Library/Keychains/System.keychain"
sha_list=$(security find-certificate -a -c "$CA_COMMON_NAME_PREFIX" -Z "$KEYCHAIN" 2>/dev/null \
           | awk '/^SHA-256 hash:/{print $3}')
if [ -z "$sha_list" ]; then
  say "   no '$CA_COMMON_NAME_PREFIX*' certificate in the System keychain"
else
  tmpdir=$(mktemp -d)
  i=0
  while IFS= read -r sha; do
    [ -z "$sha" ] && continue
    i=$((i+1))
    pem="$tmpdir/aiul-$i.pem"
    # remove-trusted-cert needs the certificate file, so export it first.
    security find-certificate -a -c "$CA_COMMON_NAME_PREFIX" -p "$KEYCHAIN" > "$pem" 2>/dev/null
    run "remove trust settings for $sha" security remove-trusted-cert -d "$pem"
    run "delete certificate $sha from the keychain" security delete-certificate -Z "$sha" "$KEYCHAIN"
  done <<< "$sha_list"
  rm -rf "$tmpdir"
fi

# ---------------------------------------------------------------------------
step "6/6  leftovers"
# The binary installed for the launch daemon, and the device token used to
# authenticate to the backend.
if [ -f /usr/local/bin/aiul ]; then
  run "remove the installed binary" rm -f /usr/local/bin/aiul
else
  say "   not present: /usr/local/bin/aiul"
fi

if dscl . -read /Users/$SERVICE_USER >/dev/null 2>&1; then
  run "remove the $SERVICE_USER service account" dscl . -delete /Users/$SERVICE_USER
  run "remove the $SERVICE_USER group" dscl . -delete /Groups/$SERVICE_USER
else
  say "   no $SERVICE_USER service account"
fi

if [ -d "$STATE_DIR" ]; then
  say "   NOTE: $STATE_DIR still holds the worker's CA copy and spooled events."
  say "   Remove it by hand if you want it gone: sudo rm -rf $STATE_DIR"
else
  say "   no $STATE_DIR"
fi

if security find-generic-password -s com.aiul.agent -a device-token >/dev/null 2>&1; then
  run "remove the device token from the keychain" security delete-generic-password -s com.aiul.agent -a device-token
else
  say "   no device token in the keychain"
fi

# ---------------------------------------------------------------------------
say ""
if [ "$DRY_RUN" -eq 1 ]; then
  say "Dry run finished. Nothing was changed."
else
  say "Kill switch finished. This Mac no longer routes traffic through aiul."
  say "Quit and reopen any terminal or app that was started while the settings were active."
fi
say ""
say "Verify by hand:"
say "  networksetup -getsecurewebproxy Wi-Fi          # Enabled: No"
say "  launchctl getenv HTTPS_PROXY                   # prints nothing"
say "  grep -c AIUL /etc/zshenv 2>/dev/null           # 0 or no such file"
say "  security find-certificate -c 'AIUL Dev Root' /Library/Keychains/System.keychain   # not found"
say "  ls /usr/local/bin/aiul                        # no such file"
say "  dscl . -read /Users/_aiul                      # record not found"
say "  curl -sI https://example.com >/dev/null && echo 'internet works'"
exit 0
