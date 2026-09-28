# Testing aiul on Ubuntu

**Ubuntu desktop (GNOME), Intel/AMD or ARM.** This installs the AI Usage Logger
agent and sends your AI usage to https://genailog.vardaam.site. You need no Go,
no Docker and no backend.

> Test devices only. The agent sets the system proxy, trusts a certificate
> authority made on your computer, and runs two background services. One
> command removes all of it (see "Removing it").

## 1. Get a device token

Ask an admin. They make one at https://genailog.vardaam.site/people ->
Device token (operating system: Ubuntu), and send you a string starting with
`aiul_`.

## 2. Download and unpack

On the GitHub Releases page, download **`aiul-linux-<version>.tar.gz`**. Not
"Source code": that is the developer repository.

```
cd ~/Downloads
tar xzf aiul-linux-<version>.tar.gz
cd aiul-linux-<version>
```

## 3. Install

```
./scripts/enroll-device.sh --token aiul_xxx
```

The script lists every change it will make and waits for your "y". It asks
for your password (sudo), and installs `libnss3-tools` if it is missing
(Chrome on Linux keeps its own list of trusted certificates).

## 4. After it finishes

- **Quit Chrome completely and reopen it.** It reads trusted certificates only
  when it starts.
- **Log out and back in.** Terminal tools read the proxy settings at login.
- Sign in to https://genailog.vardaam.site once and accept the notice. Until
  you do, your events are discarded.

Send one prompt from ChatGPT, claude.ai, Claude Code or Codex, then open
https://genailog.vardaam.site/usage.

Nothing shows up? Read the agent's log:

```
sudo tail -f /var/log/aiul/agent.err.log
```

## Removing it

```
sudo ./scripts/killswitch.sh
```

You can run it twice safely, and also on a computer where nothing is installed.

## Known gaps

- Firefox is not covered (it keeps its own list of trusted certificates).
- Tested so far in a container with systemd, not yet on a real GNOME desktop.
