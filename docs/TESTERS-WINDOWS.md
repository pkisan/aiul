# Testing aiul on Windows

**Windows 10 (1803 or later) or 11, x64.** This installs the AI Usage Logger
agent and sends your AI usage to https://genailog.vardaam.site. You need no Go,
no Docker and no backend.

> Test devices only. The agent sets the proxy for every signed-in person,
> trusts a certificate authority made on your PC, and runs two Windows
> services. One command removes all of it (see "Removing it").

## 1. Get a device token

Ask an admin. They make one at https://genailog.vardaam.site/people ->
Device token (operating system: Windows), and send you a string starting with
`aiul_`.

## 2. Download and unzip

On the GitHub Releases page, download **`aiul-windows-<version>.zip`**. Not
"Source code": that is the developer repository.

Right-click the zip -> Extract All.

## 3. Install

Open PowerShell **as Administrator** (right-click PowerShell -> Run as
administrator), then go to the extracted folder:

```
cd $HOME\Downloads\aiul-windows-<version>
Get-ChildItem -Recurse | Unblock-File
powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -Token aiul_xxx
```

`Unblock-File` removes the "downloaded from the internet" mark Windows puts on
every file in the zip. The script lists every change it will make and waits for
your "y".

## 4. After it finishes

- Sign in to https://genailog.vardaam.site once and accept the notice. Until
  you do, your events are discarded.
- Open a new PowerShell or terminal window. Windows that are already open keep
  their old settings.
- Chrome and Edge pick up the proxy by themselves. If they do not, check
  Settings -> Network & internet -> Proxy: "Use a proxy server" should be on
  with `https=127.0.0.1:8899`.

Send one prompt from ChatGPT, claude.ai, Claude Code or Codex, then open
https://genailog.vardaam.site/usage.

Nothing shows up? Read the agent's log:

```
Get-Content C:\ProgramData\AIUL\logs\agent.err.log -Tail 50 -Wait
```

## Removing it

PowerShell as Administrator, in the same folder:

```
powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1
```

You can run it twice safely, and also on a PC where nothing is installed.

## Known gaps

- `aiul.exe` is not signed yet. SmartScreen may warn about it.
- Firefox is not covered (it keeps its own list of trusted certificates).
- The git branch of terminal tools is not recorded on Windows.
- Not yet run on a real Windows PC; built and checked on a Mac only.
