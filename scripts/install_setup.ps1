# Focus Lock - consolidated one-time enforcement setup (requires admin)
#
# This is the single elevated setup step, run automatically by the bundled
# installer (build/installer/focus-lock.iss) and usable by hand. It replaces the
# manual "open an admin PowerShell and run several scripts" onboarding: one
# elevated pass provisions everything the enforcer needs.
#
# Steps (all idempotent, safe to re-run):
#   1. Add a Microsoft Defender exclusion for %APPDATA%\FocusLock so the staged
#      enforcer copy is not quarantined.
#   2. Stage the enforcer ("Ghost") exe at %APPDATA%\FocusLock\Bin\FocusLockGhost.exe
#      (copied from the installed UI exe), the fixed path the app expects
#      (backend/obfuscation/obfuscator.go).
#   3. Register the FocusLockGhost scheduled task from the SAME definition the
#      app uses (backend/scheduler/service.go): logon trigger, highest
#      privileges, AC/battery, no time limit, 1-minute auto-relaunch repetition.
#   4. Provision the tamper-protection HMAC secret in HKLM (ACL: SYSTEM +
#      Administrators only) and mark strong mode as provisioned.
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

$taskName = 'FocusLockGhost'
$flDir = Join-Path $AppData 'FocusLock'
$ghostDir = Join-Path $flDir 'Bin'
$ghostExe = Join-Path $ghostDir 'FocusLockGhost.exe'
$base = 'HKLM:\SOFTWARE\FocusLock'

# --------------------------------------------------------------------------
if ($Uninstall) {
    Write-Host "[setup] Removing Focus Lock enforcement..." -ForegroundColor Cyan
    & schtasks /delete /tn $taskName /f 2>$null
    try { Remove-Item -Path $base -Recurse -Force -ErrorAction Stop } catch {}
    try { Remove-MpPreference -ExclusionPath $flDir -ErrorAction Stop } catch {}
    try { Remove-Item -Path $flDir -Recurse -Force -ErrorAction Stop } catch {}
    Write-Host "[setup] Teardown complete." -ForegroundColor Green
    if ($Elevated) { Read-Host "`nDone. Press Enter to close" }
    return
}

# --------------------------------------------------------------------------
# 1. Defender exclusion (best-effort: may be blocked by policy / unavailable).
try {
    Add-MpPreference -ExclusionPath $flDir -ErrorAction Stop
    Write-Host "[setup] Defender exclusion added for $flDir." -ForegroundColor Green
} catch {
    Write-Warning "Could not add a Defender exclusion ($($_.Exception.Message)). Enforcement still works; add it manually if the enforcer gets quarantined."
}

# 2. Stage the enforcer exe at the fixed path the app expects.
if (-not $ExePath) { throw "Missing -ExePath (path to the installed focus-lock.exe)." }
if (-not (Test-Path $ExePath)) { throw "Installed exe not found: $ExePath" }
New-Item -ItemType Directory -Force -Path $ghostDir | Out-Null
try {
    if (Test-Path $ghostExe) {
        $old = "$ghostExe.old"
        Remove-Item $old -Force -ErrorAction SilentlyContinue
        Rename-Item $ghostExe $old -ErrorAction Stop  # a running Ghost holds the file; rename aside
    }
    Copy-Item $ExePath $ghostExe -Force
    Write-Host "[setup] Enforcer staged -> $ghostExe" -ForegroundColor Green
} catch {
    if (-not (Test-Path $ghostExe)) { throw "Could not stage the enforcer exe: $($_.Exception.Message)" }
    Write-Warning "Kept the existing enforcer exe (in use); it refreshes on the next reboot."
}

# 3. Register the enforcement task (must match taskXMLTemplate in backend/scheduler/service.go).
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
      <Command>"$ghostExe"</Command>
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
