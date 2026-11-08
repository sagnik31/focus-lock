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
#   2. Remove leftovers from older ("Ghost") builds: the "FocusLockGhost" task,
#      any enforcer still running from an old location (it holds the enforcer's
#      single-instance mutex, which would make the new one exit at once), and
#      the stale %APPDATA%\FocusLock\Bin copy.
#   3. Register the "FocusLock Enforcer" scheduled task from the SAME definition
#      the app uses (backend/scheduler/service.go): logon trigger, highest
#      privileges, AC/battery, no time limit, 1-minute auto-relaunch repetition.
#      The task runs the installed app exe itself (with --enforce) - there is no
#      separate staged or hidden copy.
#   4. Provision the tamper-protection HMAC secret in HKLM (ACL: SYSTEM +
#      Administrators only) and mark strong mode as provisioned.
#   5. Start the enforcer now through the task, so no reboot is needed.
#
# Strong mode assumes the account is a local administrator (so HighestAvailable
# runs elevated and can read the HKLM secret), and that the elevated process is
# the SAME account as the signed-in user (UAC consent, not over-the-shoulder
# credentials): the task's user SID and -AppData both come from the elevated
# account. On a standard-user account the app stays in reduced mode (per-user
# HKCU secret); everything else still works.
#
#   powershell -ExecutionPolicy Bypass -File scripts\install_setup.ps1 -ExePath <installed focus-lock.exe>
#
# Pass -Uninstall to tear everything down (used by the installer's uninstaller):
# tasks, running enforcer/app, the hosts-file block, HKLM + HKCU state, both
# Defender exclusions and %APPDATA%\FocusLock. Pass -ExePath with -Uninstall too
# so the install-folder exclusion is removed.
#
# A transcript is written to -LogPath (default %TEMP%\FocusLock-setup.log or
# FocusLock-uninstall.log). Exits non-zero if install setup fails, which the
# installer reports to the user.

param(
    [string]$ExePath,
    [string]$AppData = $env:APPDATA,
    [string]$UserSid,
    [string]$LogPath,
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
    if ($LogPath) { $argList += @('-LogPath', "`"$LogPath`"") }
    if ($Uninstall) { $argList += '-Uninstall' }
    Start-Process powershell.exe -Verb RunAs -ArgumentList $argList
    return
}

$taskName = 'FocusLock Enforcer'
$legacyTaskName = 'FocusLockGhost'        # pre-rename task from older builds
$launcherTaskName = 'FocusLockLauncher'   # legacy UAC-bypass launcher from older builds
$flDir = Join-Path $AppData 'FocusLock'
$legacyBinDir = Join-Path $flDir 'Bin'    # old staged-exe location, removed on upgrade
$base = 'HKLM:\SOFTWARE\FocusLock'
$userBase = 'HKCU:\Software\FocusLock'    # reduced-mode secret + state backup
$hostsStart = '#### FOCUS LOCK START ####' # must match backend/blocking/hosts
$hostsEnd = '#### FOCUS LOCK END ####'

# Remove a scheduled task if it exists. Do NOT use `schtasks /delete ... 2>$null`
# here: under $ErrorActionPreference='Stop', Windows PowerShell 5.1 turns the
# "task not found" stderr line into a terminating error, which aborted the
# whole script on any machine without the legacy tasks.
function Remove-TaskIfPresent([string]$Name) {
    if (Get-ScheduledTask -TaskName $Name -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $Name -Confirm:$false
        Write-Host "[setup] Removed scheduled task '$Name'."
    }
}

# Stop Focus Lock processes. With -EnforcerOnly, only enforcers (the app exe run
# with --enforce, or a legacy FocusLockGhost.exe) are stopped and the UI is left
# alone. Killing is safe: the enforcer is deliberately killable.
function Stop-FocusLock([switch]$EnforcerOnly) {
    $procs = @(Get-CimInstance Win32_Process -Filter "Name='focus-lock.exe' OR Name='FocusLockGhost.exe'" -ErrorAction SilentlyContinue)
    foreach ($p in $procs) {
        $isEnforcer = ($p.Name -ieq 'FocusLockGhost.exe') -or ($p.CommandLine -match '--enforce')
        if ($EnforcerOnly -and -not $isEnforcer) { continue }
        try {
            Stop-Process -Id $p.ProcessId -Force -ErrorAction Stop
            Write-Host "[setup] Stopped $($p.Name) (PID $($p.ProcessId))."
        } catch {
            Write-Warning "Could not stop $($p.Name) (PID $($p.ProcessId)): $($_.Exception.Message)"
        }
    }
    if ($procs.Count -gt 0) { Start-Sleep -Milliseconds 500 }   # let handles close
}

# Strip the Focus Lock section from the hosts file. A killed enforcer never runs
# its deferred Unblock, so without this an uninstall mid-lock would leave the
# blocked sites redirected to localhost forever.
function Remove-HostsBlock([string]$Path = (Join-Path $env:SystemRoot 'System32\drivers\etc\hosts')) {
    if (-not (Test-Path $Path)) { return }
    $text = [IO.File]::ReadAllText($Path)
    if (-not $text.Contains($hostsStart)) { return }
    $kept = New-Object System.Collections.Generic.List[string]
    $inBlock = $false
    foreach ($line in ($text -split "`n")) {
        $t = $line.Trim()
        if ($t -eq $hostsStart) { $inBlock = $true; continue }
        if ($t -eq $hostsEnd) { $inBlock = $false; continue }
        if (-not $inBlock) { $kept.Add($line) }
    }
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.IsReadOnly) { $item.IsReadOnly = $false }
    [IO.File]::WriteAllText($Path, ($kept -join "`n"))   # UTF-8 (no BOM), as the app writes it
    $null = & ipconfig /flushdns
    Write-Host "[setup] Removed the Focus Lock block from $Path."
}

if (-not $LogPath) {
    $LogPath = Join-Path $env:TEMP $(if ($Uninstall) { 'FocusLock-uninstall.log' } else { 'FocusLock-setup.log' })
}
try { Start-Transcript -Path $LogPath -Force | Out-Null } catch {}

$exitCode = 0
try {
    # ----------------------------------------------------------------------
    if ($Uninstall) {
        # Teardown is best-effort throughout: every step runs even if an
        # earlier one fails, so one problem can't leave the rest behind.
        Write-Host "[setup] Removing Focus Lock enforcement..." -ForegroundColor Cyan
        # Tasks first, so nothing relaunches the enforcer while we stop it.
        foreach ($t in @($taskName, $legacyTaskName, $launcherTaskName)) {
            try { Remove-TaskIfPresent $t } catch { Write-Warning "Could not remove task '$t': $($_.Exception.Message)" }
        }
        try { Stop-FocusLock } catch { Write-Warning "Could not stop Focus Lock: $($_.Exception.Message)" }
        try { Remove-HostsBlock } catch { Write-Warning "Could not clean the hosts file: $($_.Exception.Message)" }
        try { Remove-Item -Path $base -Recurse -Force -ErrorAction Stop } catch {}
        try { Remove-Item -Path $userBase -Recurse -Force -ErrorAction Stop } catch {}
        $exclusions = @($flDir)
        if ($ExePath) { $exclusions += (Split-Path -Parent $ExePath) }
        foreach ($p in $exclusions) {
            try { Remove-MpPreference -ExclusionPath $p -ErrorAction Stop; Write-Host "[setup] Defender exclusion removed for $p." } catch {}
        }
        try { Remove-Item -Path $flDir -Recurse -Force -ErrorAction Stop } catch {
            if (Test-Path $flDir) { Write-Warning "Could not fully remove ${flDir}: $($_.Exception.Message)" }
        }
        Write-Host "[setup] Teardown complete." -ForegroundColor Green
        return
    }

    # ----------------------------------------------------------------------
    # Validate the installed exe up front: the task runs it directly and the
    # Defender exclusion covers the folder it lives in.
    if (-not $ExePath) { throw "Missing -ExePath (path to the installed focus-lock.exe)." }
    if (-not (Test-Path -LiteralPath $ExePath)) { throw "Installed exe not found: $ExePath" }
    $ExePath = (Resolve-Path -LiteralPath $ExePath).Path
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

    # 2. Remove leftovers from older builds: the pre-rename task, any enforcer
    #    still running (old location or old version - it holds the
    #    Global\FocusLockGhost mutex, so the new one would exit until reboot),
    #    and the staged exe copy. Best-effort; no-ops when absent.
    Remove-TaskIfPresent $legacyTaskName
    Stop-FocusLock -EnforcerOnly
    try { Remove-Item -Path $legacyBinDir -Recurse -Force -ErrorAction Stop } catch {
        if (Test-Path $legacyBinDir) { Write-Warning "Could not remove ${legacyBinDir}: $($_.Exception.Message)" }
    }

    # 3. Register the enforcement task (must match taskXMLTemplate in
    #    backend/scheduler/service.go). The task runs the installed exe directly.
    if (-not $UserSid) { $UserSid = ([Security.Principal.WindowsIdentity]::GetCurrent()).User.Value }
    $cmdXml = [Security.SecurityElement]::Escape($ExePath)
    $sidXml = [Security.SecurityElement]::Escape($UserSid)
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
      <UserId>$sidXml</UserId>
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
      <Command>"$cmdXml"</Command>
      <Arguments>--enforce</Arguments>
    </Exec>
  </Actions>
</Task>
"@
    $tmp = Join-Path $env:TEMP 'focuslock-task.xml'
    [System.IO.File]::WriteAllText($tmp, $xml, [System.Text.Encoding]::Unicode)
    try {
        # Run schtasks with 'Continue' so a failure is reported with its own
        # message and exit code instead of PowerShell's NativeCommandError.
        $prevEap = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        $out = & schtasks /create /tn $taskName /xml $tmp /f 2>&1
        $rc = $LASTEXITCODE
        $ErrorActionPreference = $prevEap
        if ($rc -ne 0) { throw "schtasks /create failed (exit $rc): $($out -join ' ')" }
        Write-Host "[setup] Registered '$taskName' -> `"$ExePath`" --enforce (battery-safe, 1-min auto-relaunch)." -ForegroundColor Green
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

    # 5. Start the enforcer now (the logon trigger otherwise only fires at the
    #    next sign-in). Best-effort: it also starts at the next logon.
    try {
        Start-ScheduledTask -TaskName $taskName -ErrorAction Stop
        Write-Host "[setup] Started '$taskName'." -ForegroundColor Green
    } catch {
        Write-Warning "Could not start '$taskName' now ($($_.Exception.Message)); it starts at the next sign-in."
    }
    Write-Host "[setup] Done." -ForegroundColor Cyan
} catch {
    Write-Host "[setup] FAILED: $($_.Exception.Message)" -ForegroundColor Red
    $exitCode = 1
} finally {
    try { Stop-Transcript | Out-Null } catch {}
    if ($Elevated) { Read-Host "`nDone. Press Enter to close" }
}
exit $exitCode
