# Focus Lock - one-time enforcement setup (requires admin)
#
# Registers the FocusLockGhost scheduled task from the SAME definition the app
# uses (backend/scheduler/service.go): runs the enforcer at logon with highest
# privileges, on AC or battery, with no time limit, and with a 1-minute
# repetition so a killed enforcer is relaunched within ~1 minute.
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

$taskName = 'FocusLockGhost'
$exe = Join-Path $env:APPDATA 'FocusLock\Bin\FocusLockGhost.exe'
if (-not (Test-Path $exe)) {
    throw "Enforcer not found at $exe. Open Focus Lock once (or run scripts\deploy.ps1) so it is staged, then re-run this."
}
$sid = ([Security.Principal.WindowsIdentity]::GetCurrent()).User.Value

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

if ($Elevated) { Read-Host "`nDone. Press Enter to close" }
