#!/bin/bash
# tool-experiment.sh — find out whether one AI tool can be captured, and why not.
#
#   ./scripts/tool-experiment.sh cursor            # does it accept our CA?
#   ./scripts/tool-experiment.sh antigravity --research   # which hosts does it use?
#   ./scripts/tool-experiment.sh --list
#
# Two modes, and they answer different questions:
#
#   trust     (default) launches the tool with our CA named explicitly, through our
#             own proxy, and reports what the handshake did. A tool that completes
#             one can be parsed; a tool that sends an alert cannot, whatever we
#             write. This is ROADMAP step 2.
#
#   research  launches the tool through mitmweb instead, so its hosts and body
#             shapes become visible. For a tool nobody has captured yet, where the
#             allow-list does not even know its names. ROADMAP step 5.
#
# It never changes system settings. The agent must already be installed for trust
# mode; mitmweb must be running for research mode.

set -euo pipefail
cd "$(dirname "$0")/.."

CA_ROOT=/usr/local/share/aiul/root.crt
CA_BUNDLE=/usr/local/share/aiul/ca-bundle.pem
LOG=/var/log/aiul/agent.err.log
MITM_CA="$HOME/.mitmproxy/mitmproxy-ca-cert.pem"

# name | app bundle | binary | hosts to watch (regex)
TOOLS="
cursor|/Applications/Cursor.app|Cursor|cursor\.(sh|com)|cursorapi\.com
antigravity|/Applications/Antigravity IDE.app|Electron|antigravity|googleapis\.com|google\.com
vscode|/Applications/Visual Studio Code.app|Electron|githubcopilot\.com|copilot
windsurf|/Applications/Windsurf.app|Electron|codeium|windsurf
"

usage() { sed -n '2,28p' "$0" | sed 's/^# \{0,1\}//'; exit 0; }

MODE=trust
TOOL=""
while [ $# -gt 0 ]; do
  case "$1" in
    --research) MODE=research; shift ;;
    --trust)    MODE=trust; shift ;;
    --list)     printf '%s\n' "$TOOLS" | awk -F'|' 'NF{printf "  %-12s %s\n", $1, $2}'; exit 0 ;;
    -h|--help)  usage ;;
    *)          TOOL="$1"; shift ;;
  esac
done
[ -n "$TOOL" ] || usage

row="$(printf '%s\n' "$TOOLS" | grep "^$TOOL|" || true)"
[ -n "$row" ] || { echo "Unknown tool: $TOOL. Try --list." >&2; exit 2; }

APP="$(echo "$row" | cut -d'|' -f2)"
BIN="$(echo "$row" | cut -d'|' -f3)"
HOSTS="$(echo "$row" | cut -d'|' -f4-)"
HOSTS="${HOSTS//|/\\|}"
EXE="$APP/Contents/MacOS/$BIN"

[ -d "$APP" ] || { echo "Not installed: $APP" >&2; exit 1; }
[ -x "$EXE" ] || { echo "No binary at $EXE" >&2; exit 1; }

say() { printf '\n\033[1m%s\033[0m\n' "$1"; }

# A GUI app started from Finder inherits launchd's environment, not a shell's.
# Starting the binary directly is the only way to hand it a variable, which is
# the whole point of this script.
say "Quitting $TOOL so it restarts with the environment we choose"
osascript -e "quit app \"$(basename "$APP" .app)\"" 2>/dev/null || true
sleep 2

if [ "$MODE" = research ]; then
  [ -f "$MITM_CA" ] || { echo "No mitmproxy CA at $MITM_CA. Run mitmweb once first." >&2; exit 1; }
  nc -z 127.0.0.1 8080 2>/dev/null || {
    echo "Nothing is listening on 127.0.0.1:8080." >&2
    echo "Start it first:  mitmweb --listen-host 127.0.0.1 --listen-port 8080 --save-stream-file /tmp/aiul-$TOOL.flows" >&2
    exit 1
  }

  say "Launching $TOOL through mitmweb"
  echo "  every request it makes appears in the mitmweb window"
  env https_proxy=http://127.0.0.1:8080 HTTPS_PROXY=http://127.0.0.1:8080 \
      NODE_EXTRA_CA_CERTS="$MITM_CA" SSL_CERT_FILE="$MITM_CA" REQUESTS_CA_BUNDLE="$MITM_CA" \
      "$EXE" >/dev/null 2>&1 &

  cat <<NEXT

  Now use the tool: send one prompt, wait for the answer.
  Then stop mitmweb and turn the recording into a fixture:

    mitmdump -ns scripts/fixture-from-flows.py -r /tmp/aiul-$TOOL.flows \\
      --set fixture_out=agent/testdata --set fixture_case=$TOOL/conversation

  Read the .meta.json it writes: the host and path go on the allow-list only if
  they are specific to this tool (rule 3 — never a shared domain).

NEXT
  exit 0
fi

# ---- trust mode -------------------------------------------------------------
[ -f "$CA_ROOT" ] || { echo "The agent is not installed: no $CA_ROOT" >&2; exit 1; }

say "Clearing the tunnel list so this tool gets a fresh handshake"
echo "  sudo launchctl kickstart -k system/com.aiul.agent"
sudo launchctl kickstart -k system/com.aiul.agent
sleep 3
SINCE="$(date '+%Y-%m-%dT%H:%M:%S')"

say "Launching $TOOL with our CA named explicitly"
echo "  NODE_EXTRA_CA_CERTS=$CA_ROOT"
echo "  SSL_CERT_FILE=$CA_BUNDLE"
env NODE_EXTRA_CA_CERTS="$CA_ROOT" \
    SSL_CERT_FILE="$CA_BUNDLE" \
    REQUESTS_CA_BUNDLE="$CA_BUNDLE" \
    NODE_USE_SYSTEM_CA=1 \
    "$EXE" >/dev/null 2>&1 &

cat <<WAIT

  Use the tool now: send ONE prompt and wait for the answer.
  Press return here when it has replied.

WAIT
read -r _

# ---- the verdict ------------------------------------------------------------
say "What the proxy saw"
window() { awk -v s="$SINCE" -F'time=' '$2>=s' "$LOG" 2>/dev/null; }

captured="$(window | grep -E "msg=recorded" | grep -cE "$HOSTS" || true)"
alerts="$(window | grep "tunneling this host" | grep -E "$HOSTS" | grep -c "remote error: tls:" || true)"
quiet="$(window | grep "tunneling this host" | grep -E "$HOSTS" | grep -vc "remote error: tls:" || true)"
seen="$(window | grep -oE "host=[a-z0-9.-]*" | grep -E "$HOSTS" | sort | uniq -c | sort -rn | head -8 || true)"

echo "  hosts touched:"
printf '%s\n' "$seen" | sed 's/^/    /'
echo
printf '  exchanges recorded : %s\n' "$captured"
printf '  refused our CA     : %s  (a real TLS alert)\n' "$alerts"
printf '  died mid-handshake : %s  (EOF or reset — NOT a refusal)\n' "$quiet"

say "Verdict"
if [ "$captured" -gt 0 ]; then
  cat <<VERDICT
  $TOOL ACCEPTS our certificate with the CA named explicitly.
  Next: a parser for its endpoints, against a recorded fixture.
  Then work out how to hand it that variable without launching it from a shell.
VERDICT
elif [ "$alerts" -gt 0 ]; then
  cat <<VERDICT
  $TOOL REFUSES our certificate even when told where it is — it sent a TLS
  alert, which is the only evidence that counts. Metadata only until the vendor
  offers a trust setting. Record it in the capture matrix and move on.
VERDICT
else
  cat <<VERDICT
  Nothing conclusive. Either the tool made no AI request, or it reached hosts
  that are not allow-listed and so were passed through unopened. Check the hosts
  above against agent/internal/proxy/hosts.go, and if its names are missing, run
  this again with --research to find out what they are.
VERDICT
fi
