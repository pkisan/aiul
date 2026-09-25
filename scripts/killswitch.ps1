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
#   1. aiul.exe            both services (aiul, aiul-helper), and any copy started by hand
#   2. system proxy        WinINET for EVERY person (signed in or not, with admin;
#                          only you without), including the binary
#                          DefaultConnectionSettings record; WinHTTP (machine)
#   3. env vars            the machine variables listed in AIUL_MANAGED_VARS, and
#                          user/machine variables that point at us
#   4. trust               our dev root CA in CurrentUser\Root and LocalMachine\Root
#
# Only values that are clearly ours are touched: a proxy of 127.0.0.1:8899, env
# vars mentioning 8899 or AIUL. A corporate proxy or someone else's
# NODE_EXTRA_CA_CERTS is left alone.
#
# It deliberately does NOT delete %LOCALAPPDATA%\AIUL or C:\ProgramData\AIUL (the
# dev CA, the spool, the logs), nor C:\Program Files\AIUL, so a CA can be
# re-trusted instead of regenerated and the logs can still be read. Delete those
# folders by hand if you want them gone.
#
# Keep in sync with internal/platform/*_windows.go.

param([switch]$DryRun)

$ErrorActionPreference = 'Continue'   # attempt every step, like killswitch.sh

$ProxyHostPort = '127.0.0.1:8899'
$Services      = @('aiul', 'aiul-helper')   # the worker first: it asks the helper to drop the proxy
$CaNamePrefix  = 'AIUL Dev Root'
$EnvVars = @('HTTPS_PROXY','https_proxy','HTTP_PROXY','http_proxy','NO_PROXY','no_proxy',
             'NODE_EXTRA_CA_CERTS','NODE_USE_SYSTEM_CA','SSL_CERT_FILE','REQUESTS_CA_BUNDLE',
             'CODEX_CA_CERTIFICATE','CLAUDE_CODE_CERT_STORE')
$ManagedList   = 'AIUL_MANAGED_VARS'   # the names 'aiul install' set, comma-separated

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
Step '1. aiul.exe (services and hand-started copies)'
foreach ($name in $Services) {
    if (Get-Service -Name $name -ErrorAction SilentlyContinue) {
        if ($isAdmin) {
            Do-Change "stop and delete the '$name' service" {
                Stop-Service -Name $name -Force -ErrorAction SilentlyContinue
                sc.exe delete $name | Out-Null
            }
        } else { $skipped += "the '$name' service (needs admin)" }
    } else { Write-Host "   no '$name' service" }
}

$procs = Get-Process -Name 'aiul' -ErrorAction SilentlyContinue
if ($procs) {
    Do-Change "stop $($procs.Count) running aiul.exe process(es)" { $procs | Stop-Process -Force }
} else { Write-Host '   no aiul.exe running' }

# ---- 2. system proxy ---------------------------------------------------------
Step '2. system proxy'

# Turn the proxy off in one person's registry, if it is ours. $root is their
# part of the registry: HKCU for you, HKEY_USERS\<SID> for anyone else.
function Clear-Proxy($root, $label) {
    $inet = "$root\Software\Microsoft\Windows\CurrentVersion\Internet Settings"
    if (-not (Test-Path $inet)) { return }
    $server = (Get-ItemProperty -Path $inet -Name ProxyServer -ErrorAction SilentlyContinue).ProxyServer
    $conn = "$inet\Connections"
    $blob = (Get-ItemProperty -Path $conn -Name DefaultConnectionSettings -ErrorAction SilentlyContinue).DefaultConnectionSettings
    # In the binary record, byte 8 holds the flags; 0x02 is "use a proxy server".
    $blobOurs = $blob -and $blob.Length -gt 12 -and ($blob[8] -band 2) -and
                ([Text.Encoding]::ASCII.GetString($blob) -match [regex]::Escape($ProxyHostPort))
    $valueOurs = $server -and $server -match [regex]::Escape($ProxyHostPort)
    if (-not ($valueOurs -or $blobOurs)) { Write-Host "   ${label}: not ours (or none)"; return }

    Do-Change "${label}: turn off proxy $server" {
        if ($valueOurs) {
            Set-ItemProperty -Path $inet -Name ProxyEnable -Value 0
            Remove-ItemProperty -Path $inet -Name ProxyServer -ErrorAction SilentlyContinue
            Remove-ItemProperty -Path $inet -Name ProxyOverride -ErrorAction SilentlyContinue
        }
        if ($blobOurs) {
            $b = [byte[]]$blob.Clone()
            $b[8] = $b[8] -band 0xFD                                              # proxy off
            $count = [BitConverter]::GetBytes([BitConverter]::ToUInt32($b, 4) + 1) # change counter
            [Array]::Copy($count, 0, $b, 4, 4)
            Set-ItemProperty -Path $conn -Name DefaultConnectionSettings -Value $b
        }
    }
}

function Is-PersonSid($sid) {
    return ($sid -like 'S-1-5-21-*' -or $sid -like 'S-1-12-1-*') -and $sid -notlike '*_Classes'
}

if ($isAdmin) {
    # Everyone signed in: their registry is loaded under HKEY_USERS.
    $loaded = @(Get-ChildItem -Path Registry::HKEY_USERS | ForEach-Object { $_.PSChildName } | Where-Object { Is-PersonSid $_ })
    foreach ($sid in $loaded) { Clear-Proxy "Registry::HKEY_USERS\$sid" "WinINET ($sid)" }

    # Everyone signed out: load their registry file for a moment, so nobody signs
    # in later to a proxy that is not there.
    $list = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList'
    foreach ($p in Get-ChildItem -Path $list) {
        $sid = $p.PSChildName
        if (-not (Is-PersonSid $sid) -or $loaded -contains $sid) { continue }
        $hive = Join-Path ([Environment]::ExpandEnvironmentVariables($p.GetValue('ProfileImagePath'))) 'NTUSER.DAT'
        if (-not (Test-Path $hive)) { continue }
        $mount = "AIUL-$sid"
        reg.exe load "HKU\$mount" "$hive" 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) { Write-Host "   could not load $hive (in use?); skipped"; continue }
        Clear-Proxy "Registry::HKEY_USERS\$mount" "WinINET ($sid, signed out)"
        [GC]::Collect(); [GC]::WaitForPendingFinalizers()   # release handles, or unload fails
        reg.exe unload "HKU\$mount" 2>&1 | Out-Null
    }
} else {
    Clear-Proxy 'HKCU:' 'WinINET (you)'
    $skipped += "other people's proxy settings (needs admin)"
}

# Tell running programs in this session the setting changed, or they keep the
# cached proxy. Chrome and Edge also watch the registry themselves.
if (-not $DryRun) {
    Add-Type -Namespace AIUL -Name WinInet -MemberDefinition '
        [DllImport("wininet.dll", SetLastError = true)]
        public static extern bool InternetSetOption(IntPtr h, int opt, IntPtr buf, int len);' -ErrorAction SilentlyContinue
    [AIUL.WinInet]::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0) | Out-Null  # SETTINGS_CHANGED
    [AIUL.WinInet]::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0) | Out-Null  # REFRESH
}

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
$listed = @()
$managed = [Environment]::GetEnvironmentVariable($ManagedList, 'Machine')
if ($managed) { $listed = $managed -split ',' | Where-Object { $_ } }

foreach ($scope in 'User','Machine') {
    foreach ($name in ($EnvVars + $listed + $ManagedList | Select-Object -Unique)) {
        $value = [Environment]::GetEnvironmentVariable($name, $scope)
        if (-not $value) { continue }
        $ours = (Is-Ours $value) -or ($scope -eq 'Machine' -and ($listed -contains $name -or $name -eq $ManagedList))
        if (-not $ours) { continue }
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
