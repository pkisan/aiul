# Testing aiul on your Mac

**macOS only** (Apple Silicon or Intel). This installs the AI Usage Logger
agent and sends your AI usage to https://genailog.vardaam.site. You need no Go,
no Docker and no backend.

> Test devices only. The agent sets the system proxy, trusts a certificate
> authority made on your Mac, and runs two background services. One command
> removes all of it (see "Removing it").

## 1. Get a device token

Ask an admin. They make one at https://genailog.vardaam.site/people ->
Device token, and send you a string starting with `aiul_`.

## 2. Download and unzip

On the GitHub Releases page, download **`aiul-macos-<version>.zip`**. Not
"Source code": that is the developer repository.

Unzip it, then open Terminal in the unzipped folder:

```
cd ~/Downloads/aiul-macos-<version>
```

## 3. Install

```
./scripts/enroll-device.sh --token aiul_xxx
```

The script lists every change it will make and waits for your "y". It asks
for your Mac password (sudo) for the install.

## 4. After it finishes

- Sign in to https://genailog.vardaam.site once and accept the notice. Until
  you do, your events are discarded.
- Open a new Terminal window. Windows that are already open keep their old
  settings.

Send one prompt from ChatGPT, claude.ai, Claude Code or Codex, then open
https://genailog.vardaam.site/usage.

Nothing shows up? Read the agent's log:

```
tail -f /var/log/aiul/agent.err.log
```

## Removing it

```
sudo ./scripts/killswitch.sh
```

You can run it twice safely, and also on a Mac where nothing is installed.

## Known gaps

- The package is not signed yet. `sudo installer` (which the script uses)
  installs it without a Gatekeeper prompt; double-clicking the .pkg would warn.
