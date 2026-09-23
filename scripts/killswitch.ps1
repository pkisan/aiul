# killswitch.ps1 - undo EVERY system change the AI Usage Logger makes to this PC.
#
# Run it any time something feels wrong. It is idempotent: running it twice, or
# running it when nothing was ever configured, is safe and changes nothing.
#
#   powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1           # undo everything
#   powershell -ExecutionPolicy Bypass -File scripts\killswitch.ps1 -DryRun   # only print what it WOULD do
#
# Run it from an ADMINISTRATOR PowerShell to also undo the machine-wide pieces
# (service, WinHTTP proxy, machine env vars, LocalMachine trust). Without admin it
# undoes the per-user pieces and says what it skipped.
#
# It reverses, in this order:
#   1. aiul.exe            the Windows service, and any copy started by hand
#   2. system proxy        WinINET (per user) and WinHTTP (machine), only if ours
#   3. env vars            user and machine variables, only if they point at us
#   4. trust               our dev root CA in CurrentUser\Root and LocalMachine\Root
#
# Only values that are clearly ours are touched: a proxy of 127.0.0.1:8899, env
# vars mentioning 8899 or AIUL. A corporate proxy or someone else's
# NODE_EXTRA_CA_CERTS is left alone.
#
# It deliberately does NOT delete %LOCALAPPDATA%\AIUL (the dev CA and the spool),
# so a CA can be re-trusted instead of regenerated. Delete that folder by hand if
# you want the key gone.
#
# Keep in sync with internal/platform (Windows) as W3/W4 add real changes.

param([switch]$DryRun)

$ErrorActionPreference = 'Continue'   # attempt every step, like killswitch.sh

$ProxyHostPort = '127.0.0.1:8899'
$ServiceName   = 'aiul'
$CaNamePrefix  = 'AIUL Dev Root'
$EnvVars = @('HTTPS_PROXY','https_proxy','HTTP_PROXY','http_proxy','NO_PROXY','no_proxy',
             'NODE_EXTRA_CA_CERTS','NODE_USE_SYSTEM_CA','SSL_CERT_FILE','REQUESTS_CA_BUNDLE',
             'CODEX_CA_CERTIFICATE','CLAUDE_CODE_CERT_STORE')

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
           ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$skipped = @()

function Step($text) { Write-Host "`n== $text" }

# Run a change: print it, then do it unless -DryRun.
function Do-Change($description, [scriptblock]$action) {
    Write-Host "   $description"
    if ($DryRun) { Write-Host '   (dry run, not executed)'; return }
    try { & $action } catch { Write-Host "   failed: $_" }
}

function Is-Ours($value) {
    return $value -and ($value -match '8899' -or $value -match 'AIUL')
}

# ---- 1. aiul.exe -------------------------------------------------------------
Step '1. aiul.exe (service and hand-started copies)'
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    if ($isAdmin) {
        Do-Change "stop and delete the '$ServiceName' service" {
            Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
            sc.exe delete $ServiceName | Out-Null
        }
    } else { $skipped += "the '$ServiceName' service (needs admin)" }
} else { Write-Host '   no service installed' }

$procs = Get-Process -Name 'aiul' -ErrorAction SilentlyContinue
if ($procs) {
    Do-Change "stop $($procs.Count) running aiul.exe process(es)" { $procs | Stop-Process -Force }
} else { Write-Host '   no aiul.exe running' }

# ---- 2. system proxy ---------------------------------------------------------
Step '2. system proxy'
$inet = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
$server = (Get-ItemProperty -Path $inet -Name ProxyServer -ErrorAction SilentlyContinue).ProxyServer
if ($server -and $server -match [regex]::Escape($ProxyHostPort)) {
    Do-Change "WinINET (this user): turn off proxy $server" {
        Set-ItemProperty -Path $inet -Name ProxyEnable -Value 0
        Remove-ItemProperty -Path $inet -Name ProxyServer -ErrorAction SilentlyContinue
        Remove-ItemProperty -Path $inet -Name ProxyOverride -ErrorAction SilentlyContinue
        # Tell running programs the setting changed, or they keep the cached proxy.
        Add-Type -Namespace AIUL -Name WinInet -MemberDefinition '
            [DllImport("wininet.dll", SetLastError = true)]
            public static extern bool InternetSetOption(IntPtr h, int opt, IntPtr buf, int len);'
        [AIUL.WinInet]::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0) | Out-Null  # SETTINGS_CHANGED
        [AIUL.WinInet]::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0) | Out-Null  # REFRESH
    }
} else { Write-Host '   WinINET: not ours (or none)' }

$winhttp = (netsh winhttp show proxy) -join ' '
if ($winhttp -match [regex]::Escape($ProxyHostPort)) {
    if ($isAdmin) {
        Do-Change 'WinHTTP (machine): reset proxy' { netsh winhttp reset proxy | Out-Null }
    } else { $skipped += 'the WinHTTP proxy (needs admin)' }
} else { Write-Host '   WinHTTP: not ours (or none)' }

# ---- 3. env vars -------------------------------------------------------------
Step '3. environment variables'
# SetEnvironmentVariable with a target of User/Machine writes the registry AND
# broadcasts WM_SETTINGCHANGE, so new programs stop seeing the variable.
foreach ($scope in 'User','Machine') {
    foreach ($name in $EnvVars) {
        $value = [Environment]::GetEnvironmentVariable($name, $scope)
        if (-not (Is-Ours $value)) { continue }
        if ($scope -eq 'Machine' -and -not $isAdmin) { $skipped += "machine variable $name (needs admin)"; continue }
        Do-Change "$scope variable $name=$value" {
            [Environment]::SetEnvironmentVariable($name, $null, $scope)
        }
    }
}
Write-Host '   (already-open terminals keep their copy; close them)'

# ---- 4. trust ----------------------------------------------------------------
Step '4. trusted root certificates'
foreach ($store in 'Cert:\CurrentUser\Root','Cert:\LocalMachine\Root') {
    $certs = Get-ChildItem -Path $store | Where-Object { $_.Subject -like "*CN=$CaNamePrefix*" }
    if (-not $certs) { Write-Host "   ${store}: none of ours"; continue }
    if ($store -like '*LocalMachine*' -and -not $isAdmin) { $skipped += "$store (needs admin)"; continue }
    foreach ($c in $certs) {
        Do-Change "remove $($c.Subject) from $store" { Remove-Item -Path "$store\$($c.Thumbprint)" }
    }
}

# ---- summary -----------------------------------------------------------------
Write-Host ''
if ($skipped.Count) {
    Write-Host 'SKIPPED, run again from an Administrator PowerShell:'
    $skipped | ForEach-Object { Write-Host "   - $_" }
    exit 1
}
if ($DryRun) { Write-Host 'Dry run: nothing was changed. Run again without -DryRun to do the above.'; exit 0 }
Write-Host 'Done. Nothing of the AI Usage Logger is left active on this PC.'
