# enroll-device.ps1 - put the agent on this Windows PC and point it at a backend.
# The Windows twin of enroll-device.sh.
#
# Run from an ADMINISTRATOR PowerShell, in the repository folder:
#
#   powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1                            # backend on this PC
#   powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -User admin@example.com    # ...linked to that person
#   powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -Endpoint https://aiul.example.com/api/aiul/events -Token aiul_xxx
#   powershell -ExecutionPolicy Bypass -File scripts\enroll-device.ps1 -Uninstall
#
# THIS ONE CHANGES THE MACHINE. It installs two Windows services, trusts a
# locally generated CA, sets machine environment variables and points every
# signed-in person's proxy at 127.0.0.1:8899. Everything it does is undone by:
#
#   powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1
#
# It asks before changing anything, and shows every step first.

param(
    [string]$Endpoint = '',
    [string]$Token = '',
    [string]$User = '',
    [switch]$Uninstall
)

$ErrorActionPreference = 'Stop'
$Repo = Split-Path -Parent $PSScriptRoot
$Port = if ($env:AIUL_PORT) { $env:AIUL_PORT } else { '8088' }
$Config = 'C:\ProgramData\AIUL\agent.conf'

function Say($text) { Write-Host "`n$text" -ForegroundColor Cyan }

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
           ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host 'Run this from an Administrator PowerShell (right-click PowerShell > Run as administrator).' -ForegroundColor Red
    exit 1
}

if ($Uninstall) {
    Say 'Removing the agent'
    & "$Repo\scripts\killswitch.ps1"
    exit $LASTEXITCODE
}

# ---- 1. the binary -----------------------------------------------------------
# dist\aiul.exe, built on the Mac by scripts/build.sh and copied here, or built
# here if Go is installed.
$Bin = Join-Path $Repo 'dist\aiul.exe'
if (-not (Test-Path $Bin)) {
    if (Get-Command go -ErrorAction SilentlyContinue) {
        Say 'Building the agent'
        New-Item -ItemType Directory -Force -Path (Join-Path $Repo 'dist') | Out-Null
        Push-Location (Join-Path $Repo 'agent')
        $env:CGO_ENABLED = '0'
        go build -trimpath -ldflags '-s -w' -o $Bin ./cmd/aiul
        Pop-Location
    } else {
        Write-Host "No $Bin and no Go to build it." -ForegroundColor Red
        Write-Host 'On the Mac: ./scripts/build.sh, then copy dist/aiul.exe into dist\ here.'
        exit 1
    }
}
Write-Host "Agent: $Bin ($(& $Bin version))"

# ---- 2. where events go --------------------------------------------------------
if (-not $Endpoint) { $Endpoint = "http://127.0.0.1:$Port/api/aiul/events" }

# ---- 3. a device token ---------------------------------------------------------
# Provisioned against the Docker backend on this PC when none was given. Someone
# else's backend means asking them for a token.
if (-not $Token) {
    if ($Endpoint -notmatch '^http://(127\.0\.0\.1|localhost)') {
        Write-Host 'A remote endpoint needs a token from whoever runs that backend:' -ForegroundColor Red
        Write-Host '  php artisan aiul:provision-device <pc-name> --tenant=<slug> --platform=windows'
        Write-Host 'Then pass it here with -Token.'
        exit 1
    }
    Say 'Provisioning this device with the local backend'
    $provision = @('php', 'artisan', 'aiul:provision-device', $env:COMPUTERNAME, '--tenant=dev', '--platform=windows')
    if ($User) { $provision += "--user=$User" }
    $out = & docker compose -f (Join-Path $Repo 'compose.demo.yaml') exec -T app @provision 2>&1 | Out-String
    if ($out -match '(aiul_[A-Za-z0-9_-]+)') { $Token = $Matches[1] }
    if (-not $Token) {
        Write-Host 'Could not obtain a device token. Is Docker Desktop running, and the backend up?' -ForegroundColor Red
        Write-Host '  docker compose -f compose.demo.yaml up -d --build'
        Write-Host $out
        exit 1
    }
}

# ---- 4. show everything before touching the machine ------------------------------
Say 'About to change this PC'
Write-Host @"
  1. write $Config  (SYSTEM, Administrators and the agent only)
         AIUL_ENDPOINT=$Endpoint
         AIUL_DEVICE_TOKEN=aiul_... (hidden)
  2. set AIUL_DEV_ALLOW_UNMANAGED=1 for the install
         Windows MDM detection is not built yet, and the agent refuses to run
         on an unmanaged device without this override. For testing only.
  3. aiul.exe ca ensure, then aiul.exe install --apply
         generates a CA for THIS PC and trusts it (LocalMachine\Root), sets
         machine environment variables, starts two services (aiul-helper,
         aiul), and points every signed-in person's proxy at 127.0.0.1:8899.
         The full list of commands is printed before it runs.

  Undo all of it, at any time, with:
         powershell -ExecutionPolicy Bypass -File $Repo\scripts\killswitch.ps1
"@
$answer = Read-Host 'Proceed? [y/N]'
if ($answer -notmatch '^(y|yes)$') { Write-Host 'Nothing was changed.'; exit 0 }

# ---- 5. do it ------------------------------------------------------------------
Say "Writing $Config"
New-Item -ItemType Directory -Force -Path (Split-Path $Config) | Out-Null
Set-Content -Path $Config -Encoding ascii -Value @("AIUL_ENDPOINT=$Endpoint", "AIUL_DEVICE_TOKEN=$Token")
# Lock it at once: ProgramData lets every user read by default. The install adds
# read access for the agent's own service account.
icacls $Config /inheritance:r /grant:r '*S-1-5-18:F' /grant:r '*S-1-5-32-544:F' | Out-Null

Say 'Installing the agent'
$env:AIUL_DEV_ALLOW_UNMANAGED = '1'
$ErrorActionPreference = 'Continue'   # native commands report through exit codes
& $Bin ca ensure
if ($LASTEXITCODE -ne 0) { Write-Host 'Could not provision a usable CA.' -ForegroundColor Red; exit 1 }
& $Bin install --apply --yes
if ($LASTEXITCODE -ne 0) {
    Write-Host 'The install failed and rolled itself back. The worker log may say why:' -ForegroundColor Red
    Get-Content 'C:\ProgramData\AIUL\logs\agent.err.log' -Tail 20 -ErrorAction SilentlyContinue
    exit 1
}

Say 'Checking it came up'
& 'C:\Program Files\AIUL\aiul.exe' status

$usage = $Endpoint -replace '/api/aiul/events$', '/usage'
Write-Host @"

  Open a NEW PowerShell or terminal for the environment variables to apply;
  windows that were already open keep their old copy.

  Events are only kept once this device belongs to a person who accepted the
  notice: sign in to the dashboard as that person once and accept it (or run
  'aiul login' if no -User was given).

  Send one prompt from ChatGPT in Chrome or Edge, Claude Code or Codex, then look at:

      $usage

  If nothing appears, the logs say why:

      Get-Content C:\ProgramData\AIUL\logs\agent.err.log -Tail 50 -Wait
      Get-Content C:\ProgramData\AIUL\logs\helper.err.log -Tail 50

  Remove everything:

      powershell -ExecutionPolicy Bypass -File $Repo\scripts\killswitch.ps1
"@
