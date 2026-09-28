# Testing aiul on your machine

This installs the AI Usage Logger agent and sends your AI usage to
https://genailog.vardaam.site. You need no Go, no Docker and no backend.

> Test devices only. The agent sets the system proxy, trusts a certificate
> authority made on your machine, and runs two background services. One
> command removes all of it (see "Removing it").

## 1. Get a device token

Ask an admin. They make one at https://genailog.vardaam.site/people ->
Device token, and send you a string starting with `aiul_`.

## 2. Download and unzip

Download `aiul-<version>.zip` from the latest release on the GitHub Releases
page and unzip it. Open a terminal in the unzipped folder.

## 3. Install

macOS or Ubuntu:

```
./scripts/enroll-device.sh --token aiul_xxx
```

Windows (PowerShell, run as Administrator):

```
Get-ChildItem -Recurse | Unblock-File
powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -Token aiul_xxx
```

The script lists every change it will make and waits for your "y".

## 4. After it finishes

- Sign in to https://genailog.vardaam.site once and accept the notice. Until
  you do, your events are discarded.
- Open a new terminal. Terminals that are already open keep their old settings.
- Ubuntu: quit Chrome completely and reopen it, then log out and back in.

Send one prompt from ChatGPT, claude.ai, Claude Code or Codex, then open
https://genailog.vardaam.site/usage.

Nothing shows up? Read the agent's log:

- macOS, Ubuntu: `tail -f /var/log/aiul/agent.err.log`
- Windows: `Get-Content C:\ProgramData\AIUL\logs\agent.err.log -Tail 50 -Wait`

## Removing it

- macOS, Ubuntu: `sudo ./scripts/killswitch.sh`
- Windows (Administrator): `powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1`

You can run it twice safely, and also on a machine where nothing is installed.

## Known gaps

- Nothing is signed yet. macOS installs the package with `sudo installer`
  without a Gatekeeper prompt. Windows SmartScreen may warn about `aiul.exe`.
- Firefox is not covered on Windows or Ubuntu.
