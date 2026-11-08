# Focus Lock - one-time enforcement setup (requires admin)
#
# Registers the "FocusLock Enforcer" scheduled task from the SAME definition the
# app uses (backend/scheduler/service.go): runs the enforcer (the installed app
# exe, with --enforce) at logon with highest privileges, on AC or battery, with
# no time limit, and a 1-minute repetition so a killed enforcer is relaunched
# within ~1 minute. On upgrade it removes the old "FocusLockGhost" task.
#
# It also provisions the tamper-protection secret in HKLM so the config's HMAC
# key is no longer readable by the (non-admin) user it enforces. The secret
# lives in HKLM\SOFTWARE\FocusLock\Security with an ACL that grants only SYSTEM
# and Administrators: the elevated Ghost (this task, run with highest privilege)
# can read it, but the medium-integrity UI cannot, so the user can no longer read
# the key and forge a config. This turns on "strong mode"; without it the app
# falls back to the per-user HKCU secret ("reduced mode").
#
# NOTE: strong mode assumes the account is a local administrator (so the
# HighestAvailable task actually runs elevated). On a standard-user account the
# Ghost can't read the secret and the app stays in reduced mode.
#
# The app can't register this itself: the UI runs non-elevated and a
# highest-privilege task needs admin (schtasks returns "Access is denied"
# otherwise). Run this ONCE; it prompts for elevation (UAC) if needed. Re-run it
# after upgrading from an older build whose task stopped on battery / had no
# auto-relaunch.
#
#   powershell -ExecutionPolicy Bypass -File scripts\setup_persistence.ps1

param([switch]$Elevated)

$ErrorActionPreference = 'Stop'

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Re-launch elevated if we aren't admin yet.
if (-not (Test-Admin)) {
    if ($Elevated) { throw "Elevation did not grant administrator rights." }
    Write-Host "Requesting administrator privileges (UAC)..." -ForegroundColor Cyan
    Start-Process powershell.exe -Verb RunAs -ArgumentList @(
        '-NoProfile', '-ExecutionPolicy', 'Bypass',
        '-File', "`"$PSCommandPath`"", '-Elevated')
    return
}

$taskName = 'FocusLock Enforcer'
$legacyTaskName = 'FocusLockGhost'   # pre-rename task from older builds
$exe = Join-Path $env:APPDATA 'FocusLock\focus-lock.exe'
if (-not (Test-Path $exe)) {
    throw "Focus Lock exe not found at $exe. Run scripts\deploy.ps1 (or install the app) so it is in place, then re-run this."
}
$sid = ([Security.Principal.WindowsIdentity]::GetCurrent()).User.Value

# Remove leftovers from older ("Ghost") builds: the pre-rename task and the
# staged exe copy. Best-effort; a no-op when they don't exist.
& schtasks /delete /tn $legacyTaskName /f 2>$null
try { Remove-Item -Path (Join-Path $env:APPDATA 'FocusLock\Bin') -Recurse -Force -ErrorAction Stop } catch {}

# Must match taskXMLTemplate in backend/scheduler/service.go.
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
      <UserId>$sid</UserId>
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
      <Command>"$exe"</Command>
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
    Write-Host "[setup] Reboot to start the new enforcer and clear the old one." -ForegroundColor Cyan
}
finally {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
}

# --- Strong-mode secret provisioning ---------------------------------------
# Store the HMAC secret where the enforced user cannot read it. The elevated
# Ghost reads it (Administrators enabled in its task token); the medium UI
# cannot (Administrators is filtered/disabled in its token).
$base = 'HKLM:\SOFTWARE\FocusLock'
$secKey = Join-Path $base 'Security'

if (-not (Test-Path $base)) { New-Item -Path $base -Force | Out-Null }
if (-not (Test-Path $secKey)) { New-Item -Path $secKey -Force | Out-Null }

# Harden the Security subkey ACL: no inheritance, only SYSTEM + Administrators.
$acl = Get-Acl -Path $secKey
$acl.SetAccessRuleProtection($true, $false)   # protect; do not copy inherited ACEs
foreach ($rule in @($acl.Access)) { [void]$acl.RemoveAccessRule($rule) }
$adminsSid = New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-544')
$systemSid = New-Object System.Security.Principal.SecurityIdentifier('S-1-5-18')
$inherit = [System.Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit'
$prop = [System.Security.AccessControl.PropagationFlags]::None
$allow = [System.Security.AccessControl.AccessControlType]::Allow
$acl.AddAccessRule((New-Object System.Security.AccessControl.RegistryAccessRule($adminsSid, 'FullControl', $inherit, $prop, $allow)))
$acl.AddAccessRule((New-Object System.Security.AccessControl.RegistryAccessRule($systemSid, 'FullControl', $inherit, $prop, $allow)))
Set-Acl -Path $secKey -AclObject $acl

# Generate the secret ONLY if absent — regenerating would invalidate every
# config/backup already signed with the old key.
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

# Mark strong mode as provisioned (readable by the UI so it knows to use the
# request channel instead of signing).
New-ItemProperty -Path $base -Name 'Provisioned' -PropertyType DWord -Value 1 -Force | Out-Null
Write-Host "[setup] Strong mode enabled (config signing moved to the elevated enforcer)." -ForegroundColor Cyan

if ($Elevated) { Read-Host "`nDone. Press Enter to close" }
