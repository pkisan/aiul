# Installing the agent on a device (live site: genailog.vardaam.site)

No Docker. The device talks to https://genailog.vardaam.site directly.

## Admin, once per device

1. **People** → the person → **Device token** → device name (what `hostname`
   prints on their computer) and OS → **Create token**. Send it privately.
2. The person must sign in once at https://genailog.vardaam.site and **accept
   the notice**. Until they do, their device's events are received and thrown
   away.

## On the device (in the repository folder)

| OS | Install | Remove |
| --- | --- | --- |
| macOS / Ubuntu | `./scripts/enroll-device.sh --token aiul_xxx` | `sudo ./scripts/killswitch.sh` (Ubuntu: `killswitch-linux.sh`) |
| Windows (Administrator PowerShell) | `powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -Token aiul_xxx` | `powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1` |

The macOS installer uses the newest `dist/aiul-*.pkg` (Windows `dist\aiul.exe`,
Ubuntu `dist/aiul-linux-*`); copy it into `dist/` or have Go installed.

## Starting over on a device that had the Docker setup

```bash
sudo ./scripts/killswitch.sh                     # removes agent, proxy, CA, services
docker compose -f compose.demo.yaml down         # optional: stop the local backend
./scripts/enroll-device.sh --token aiul_xxx      # fresh token from People
```

## Check

Send one ChatGPT / Claude prompt, then open https://genailog.vardaam.site/usage.
Nothing? On the device:

```bash
tail -50 /var/log/aiul/agent.err.log                                         # macOS / Ubuntu
Get-Content C:\ProgramData\AIUL\logs\agent.err.log -Tail 50                  # Windows
```

and on the server: `grep "Discarded events" storage/logs/laravel-*.log | tail`
(means the person has not accepted the notice).
