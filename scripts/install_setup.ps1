# Focus Lock - consolidated one-time enforcement setup (requires admin)
#
# This is the single elevated setup step, run automatically by the bundled
# installer (installer/focus-lock.iss) and usable by hand. It replaces the
# manual "open an admin PowerShell and run several scripts" onboarding: one
# elevated pass provisions everything the enforcer needs.
#
# Steps (all idempotent, safe to re-run):
#   1. Add Microsoft Defender exclusions for the app's install folder and
#      %APPDATA%\FocusLock so the enforcer exe isn't quarantined.
#   2. Register the "FocusLock Enforcer" scheduled task from the SAME definition
#      the app uses (backend/scheduler/service.go): logon trigger, highest
#      privileges, AC/battery, no time limit, 1-minute auto-relaunch repetition.
#      The task runs the installed app exe itself (with --enforce) - there is no
#      separate staged or hidden copy.
#   3. Provision the tamper-protection HMAC secret in HKLM (ACL: SYSTEM +
#      Administrators only) and mark strong mode as provisioned.
#
# On upgrade it also removes the old "FocusLockGhost" task and the stale
# %APPDATA%\FocusLock\Bin copy left by older builds.
#
# Strong mode assumes the account is a local administrator (so HighestAvailable
# runs elevated and can read the HKLM secret). On a standard-user account the
# app stays in reduced mode (per-user HKCU secret); everything else still works.
#
#   powershell -ExecutionPolicy Bypass -File scripts\install_setup.ps1 -ExePath <installed focus-lock.exe>
#
# Pass -Uninstall to tear everything down (used by the installer's uninstaller).

param(
    [string]$ExePath,
    [string]$AppData = $env:APPDATA,
    [string]$UserSid,
    [switch]$Uninstall,
    [switch]$Elevated
)

$ErrorActionPreference = 'Stop'

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Re-launch elevated if needed (no-op when the installer already runs us as admin).
if (-not (Test-Admin)) {
    if ($Elevated) { throw "Elevation did not grant administrator rights." }
    $argList = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"", '-Elevated')
    if ($ExePath) { $argList += @('-ExePath', "`"$ExePath`"") }
    if ($AppData) { $argList += @('-AppData', "`"$AppData`"") }
    if ($UserSid) { $argList += @('-UserSid', "`"$UserSid`"") }
    if ($Uninstall) { $argList += '-Uninstall' }
    Start-Process powershell.exe -Verb RunAs -ArgumentList $argList
    return
}

$taskName = 'FocusLock Enforcer'
$legacyTaskName = 'FocusLockGhost'   # pre-rename task from older builds
$flDir = Join-Path $AppData 'FocusLock'
$legacyBinDir = Join-Path $flDir 'Bin'   # old staged-exe location, removed on upgrade
$base = 'HKLM:\SOFTWARE\FocusLock'

# --------------------------------------------------------------------------
if ($Uninstall) {
    Write-Host "[setup] Removing Focus Lock enforcement..." -ForegroundColor Cyan
    & schtasks /delete /tn $taskName /f 2>$null
    & schtasks /delete /tn $legacyTaskName /f 2>$null
    & schtasks /delete /tn 'FocusLockLauncher' /f 2>$null   # legacy UAC-bypass launcher from older builds
    try { Remove-Item -Path $base -Recurse -Force -ErrorAction Stop } catch {}
    try { Remove-MpPreference -ExclusionPath $flDir -ErrorAction Stop } catch {}
    try { Remove-Item -Path $flDir -Recurse -Force -ErrorAction Stop } catch {}
    Write-Host "[setup] Teardown complete." -ForegroundColor Green
    if ($Elevated) { Read-Host "`nDone. Press Enter to close" }
    return
}

# --------------------------------------------------------------------------
# Validate the installed exe up front: the task runs it directly and the
# Defender exclusion covers the folder it lives in.
if (-not $ExePath) { throw "Missing -ExePath (path to the installed focus-lock.exe)." }
if (-not (Test-Path $ExePath)) { throw "Installed exe not found: $ExePath" }
$exeDir = Split-Path -Parent $ExePath

# 1. Defender exclusions (best-effort: may be blocked by policy / unavailable).
#    Exclude the install folder (the enforcer exe) and the data folder.
foreach ($p in @($exeDir, $flDir)) {
    try {
        Add-MpPreference -ExclusionPath $p -ErrorAction Stop
        Write-Host "[setup] Defender exclusion added for $p." -ForegroundColor Green
    } catch {
        Write-Warning "Could not add a Defender exclusion for $p ($($_.Exception.Message)). Enforcement still works; add it manually if the enforcer gets quarantined."
    }
}

# 2. Remove leftovers from older ("Ghost") builds: the pre-rename task and the
#    staged exe copy. Best-effort; a no-op when they don't exist.
& schtasks /delete /tn $legacyTaskName /f 2>$null
try { Remove-Item -Path $legacyBinDir -Recurse -Force -ErrorAction Stop } catch {}

# 3. Register the enforcement task (must match taskXMLTemplate in
#    backend/scheduler/service.go). The task runs the installed exe directly.
if (-not $UserSid) { $UserSid = ([Security.Principal.WindowsIdentity]::GetCurrent()).User.Value }
$xml = @"
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>PT1M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>$UserSid</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>"$ExePath"</Command>
      <Arguments>--enforce</Arguments>
    </Exec>
  </Actions>
</Task>
"@
$tmp = Join-Path $env:TEMP 'focuslock-task.xml'
[System.IO.File]::WriteAllText($tmp, $xml, [System.Text.Encoding]::Unicode)
try {
    & schtasks /create /tn $taskName /xml $tmp /f
    if ($LASTEXITCODE -ne 0) { throw "schtasks exited with code $LASTEXITCODE" }
    Write-Host "[setup] Registered '$taskName' (battery-safe, 1-min auto-relaunch)." -ForegroundColor Green
} finally {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
}

# 4. Strong-mode secret provisioning (HKLM, ACL: SYSTEM + Administrators only).
$secKey = Join-Path $base 'Security'
if (-not (Test-Path $base)) { New-Item -Path $base -Force | Out-Null }
if (-not (Test-Path $secKey)) { New-Item -Path $secKey -Force | Out-Null }

$acl = Get-Acl -Path $secKey
$acl.SetAccessRuleProtection($true, $false)
foreach ($rule in @($acl.Access)) { [void]$acl.RemoveAccessRule($rule) }
$adminsSid = New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-544')
$systemSid = New-Object System.Security.Principal.SecurityIdentifier('S-1-5-18')
$inherit = [System.Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit'
$prop = [System.Security.AccessControl.PropagationFlags]::None
$allow = [System.Security.AccessControl.AccessControlType]::Allow
$acl.AddAccessRule((New-Object System.Security.AccessControl.RegistryAccessRule($adminsSid, 'FullControl', $inherit, $prop, $allow)))
$acl.AddAccessRule((New-Object System.Security.AccessControl.RegistryAccessRule($systemSid, 'FullControl', $inherit, $prop, $allow)))
Set-Acl -Path $secKey -AclObject $acl

# Generate the secret only if absent (regenerating invalidates every signed config/backup).
$existing = (Get-ItemProperty -Path $secKey -Name 'SecretKey' -ErrorAction SilentlyContinue).SecretKey
if ([string]::IsNullOrEmpty($existing)) {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $hex = ($bytes | ForEach-Object { $_.ToString('x2') }) -join ''
    New-ItemProperty -Path $secKey -Name 'SecretKey' -PropertyType String -Value $hex -Force | Out-Null
    Write-Host "[setup] Generated tamper-protection secret (strong mode)." -ForegroundColor Green
} else {
    Write-Host "[setup] Existing tamper-protection secret preserved." -ForegroundColor Green
}
New-ItemProperty -Path $base -Name 'Provisioned' -PropertyType DWord -Value 1 -Force | Out-Null
Write-Host "[setup] Done. Reboot (or sign out/in) to start the enforcer." -ForegroundColor Cyan

if ($Elevated) { Read-Host "`nDone. Press Enter to close" }
