# Focus Lock - consolidated one-time enforcement setup (requires admin)
#
# This is the single elevated setup step, run automatically by the bundled
# installer (installer/focus-lock.iss) and usable by hand. It replaces the
# manual "open an admin PowerShell and run several scripts" onboarding: one
# elevated pass provisions everything the enforcer needs.
#
# Steps (all idempotent, safe to re-run):
#   1. Add a Microsoft Defender exclusion for the app's install folder so the
#      enforcer exe isn't quarantined.
#   2. Remove leftovers from older ("Ghost") builds: the "FocusLockGhost" task,
#      any enforcer still running from an old location (it holds the enforcer's
#      single-instance mutex, which would make the new one exit at once), and
#      the stale %APPDATA%\FocusLock\Bin copy.
#   3. Provision the tamper-protection HMAC secret in HKLM (ACL: SYSTEM +
#      Administrators only) and mark strong mode as provisioned.
#      Also create %ProgramData%\FocusLock with the same SYSTEM + Administrators
#      only ACL: the enforcer keeps an admin-only copy of the signed config
#      there, so deleting or replacing the files in %APPDATA% can't change an
#      active session (backend/storage/securebackup_windows.go).
#   4. Register the "FocusLock Enforcer" scheduled task from the SAME definition
#      the app uses (backend/scheduler/service.go): logon + 1-minute time trigger, highest
#      privileges, AC/battery, no time limit, 1-minute auto-relaunch repetition.
#      The task runs the installed app exe itself (with --enforce) - there is no
#      separate staged or hidden copy.
#   4b. Register the on-demand "FocusLock Updater" task, which installs in-app
#      updates without a UAC prompt (best-effort; see docs/adr/0009).
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
# tasks, running enforcer/app, the hosts-file block, HKLM + HKCU state, the
# Defender exclusion and %APPDATA%\FocusLock. Pass -ExePath with -Uninstall too
# so the install-folder exclusion is removed.
#
# A transcript is written to -LogPath (default %TEMP%\FocusLock-setup.log or
# FocusLock-uninstall.log). Exits non-zero if install setup fails, which the
# installer reports to the user (2 = skipped: UAC was approved by another account).

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
$updaterTaskName = 'FocusLock Updater'    # no-prompt in-app updates (ADR 0009; backend/updater/apply.go)
$legacyTaskName = 'FocusLockGhost'        # pre-rename task from older builds
$launcherTaskName = 'FocusLockLauncher'   # legacy UAC-bypass launcher from older builds
$flDir = Join-Path $AppData 'FocusLock'
$legacyBinDir = Join-Path $flDir 'Bin'    # old staged-exe location, removed on upgrade
$base = 'HKLM:\SOFTWARE\FocusLock'
$userBase = 'HKCU:\Software\FocusLock'    # reduced-mode secret + state backup
$secureDir = Join-Path $env:ProgramData 'FocusLock' # admin-only signed config copy
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

# The lines without the Focus Lock section. When the end marker is missing, only
# lines Focus Lock wrote are removed. Must match stripBlock in
# backend/blocking/hosts/manager.go.
function Remove-FocusLockLines([string[]]$Lines) {
    $kept = New-Object System.Collections.Generic.List[string]
    $t = @($Lines | ForEach-Object { $_.Trim() })
    $n = $t.Count
    $i = 0
    while ($i -lt $n) {
        if ($t[$i] -ceq $hostsEnd) { $i++; continue }
        if ($t[$i] -cne $hostsStart) { $kept.Add($Lines[$i]); $i++; continue }
        $j = $i + 1
        while ($j -lt $n -and $t[$j] -cne $hostsStart -and $t[$j] -cne $hostsEnd) { $j++ }
        if ($j -lt $n -and $t[$j] -ceq $hostsEnd) { $i = $j + 1; continue }
        $i++
        while ($i -lt $j) {
            $ours = $t[$i].StartsWith('127.0.0.1 ', [StringComparison]::Ordinal)
            $pair = if ($ours) { '::1 ' + $t[$i].Substring(10) } else { '' }
            if ($ours -and $i + 1 -lt $j -and ($t[$i + 1] -ceq $pair -or
                    ($i + 2 -eq $n -and $pair.StartsWith($t[$i + 1], [StringComparison]::Ordinal)))) {
                $i += 2
            } elseif ($i + 1 -eq $n -and ($ours -or '127.0.0.1'.StartsWith($t[$i], [StringComparison]::Ordinal))) {
                $i++
            } else {
                break
            }
        }
    }
    , $kept.ToArray()
}

# Strip the Focus Lock section from the hosts file. A killed enforcer never runs
# its deferred Unblock, so without this an uninstall mid-lock would leave the
# blocked sites redirected to localhost forever.
function Remove-HostsBlock([string]$Path = (Join-Path $env:SystemRoot 'System32\drivers\etc\hosts')) {
    if (-not (Test-Path $Path)) { return }
    $text = [IO.File]::ReadAllText($Path)
    if (-not $text.Contains($hostsStart)) { return }
    $kept = Remove-FocusLockLines ($text -split "`n")
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.IsReadOnly) { $item.IsReadOnly = $false }
    [IO.File]::WriteAllText($Path, ($kept -join "`n"))   # UTF-8 (no BOM), as the app writes it
    $null = & ipconfig /flushdns
    Write-Host "[setup] Removed the Focus Lock block from $Path."
}

# Remove the Chrome URLBlocklist entries Focus Lock added (recorded in
# HKLM\SOFTWARE\FocusLock\ChromeBlocklistOwned; must match
# backend/blocking/browserpolicy). Entries the user had are kept; the policy
# key is deleted when nothing is left, so Chrome stops showing "Managed by
# your organization". Must run before $base is deleted.
function Remove-ChromePolicy {
    $owned = @((Get-ItemProperty -Path $base -Name 'ChromeBlocklistOwned' -ErrorAction SilentlyContinue).ChromeBlocklistOwned |
        Where-Object { $_ })
    if ($owned.Count -eq 0) { return }
    $chromeKey = 'HKLM:\SOFTWARE\Policies\Google\Chrome'
    $listKey = "$chromeKey\URLBlocklist"
    if (Test-Path $listKey) {
        # Only numbered values are list entries; anything else there is left alone.
        $key = Get-Item -Path $listKey
        $names = @($key.Property | Where-Object { $_ -match '^\d+$' } | Sort-Object { [int]$_ })
        $kept = @($names | ForEach-Object { $key.GetValue($_) } | Where-Object { $owned -notcontains $_ })
        for ($i = 0; $i -lt $kept.Count; $i++) {
            $null = New-ItemProperty -Path $listKey -Name ([string]($i + 1)) -Value $kept[$i] -PropertyType String -Force
        }
        $want = @(for ($i = 1; $i -le $kept.Count; $i++) { [string]$i })
        foreach ($name in $names) {
            if ($want -notcontains $name) { Remove-ItemProperty -Path $listKey -Name $name }
        }
        foreach ($k in @($listKey, $chromeKey)) {
            if (-not (Test-Path $k) -or (Get-ChildItem $k) -or (Get-Item $k).Property) { break }
            Remove-Item -Path $k -Force
        }
    }
    Remove-ItemProperty -Path $base -Name 'ChromeBlocklistOwned' -ErrorAction SilentlyContinue
    Write-Host "[setup] Removed Focus Lock's Chrome URLBlocklist entries."
}

# Whether the account running this (elevated) script is the one signed in to
# this desktop session, judged by who owns the session's Explorer shell. True
# when there is no shell to ask.
function Test-ElevatedIsSignedInUser {
    $me = ([Security.Principal.WindowsIdentity]::GetCurrent()).User.Value
    $session = [Diagnostics.Process]::GetCurrentProcess().SessionId
    $owners = @(Get-CimInstance Win32_Process -Filter "Name='explorer.exe' AND SessionId=$session" -ErrorAction SilentlyContinue |
        ForEach-Object { (Invoke-CimMethod -InputObject $_ -MethodName GetOwnerSid -ErrorAction SilentlyContinue).Sid } |
        Where-Object { $_ })
    ($owners.Count -eq 0) -or ($owners -contains $me)
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
        foreach ($t in @($taskName, $updaterTaskName, $legacyTaskName, $launcherTaskName)) {
            try { Remove-TaskIfPresent $t } catch { Write-Warning "Could not remove task '$t': $($_.Exception.Message)" }
        }
        try { Stop-FocusLock } catch { Write-Warning "Could not stop Focus Lock: $($_.Exception.Message)" }
        try { Remove-HostsBlock } catch { Write-Warning "Could not clean the hosts file: $($_.Exception.Message)" }
        try { Remove-ChromePolicy } catch { Write-Warning "Could not clean the Chrome policy: $($_.Exception.Message)" }
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
        try { Remove-Item -Path $secureDir -Recurse -Force -ErrorAction Stop } catch {
            if (Test-Path $secureDir) { Write-Warning "Could not fully remove ${secureDir}: $($_.Exception.Message)" }
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

    # 0. Strong mode works only when the elevated account is the signed-in user:
    #    the signer runs as, and keeps its config in the profile of, the account
    #    that approved UAC. With another account's credentials, stop here and
    #    change nothing: the app then runs in reduced mode (exit code 2).
    if (-not (Test-ElevatedIsSignedInUser)) {
        Write-Warning "Administrator approval came from a different account than the signed-in user. Skipping strong-mode setup; Focus Lock runs in reduced mode for this user."
        exit 2   # not 'return', which would skip the exit code below
    }

    # 1. Defender exclusion for the install folder (best-effort: may be blocked by
    #    policy / unavailable). Not for the data folder, which any program can
    #    write to: older versions excluded it, so that is undone here.
    if ($flDir.TrimEnd('\') -ine $exeDir.TrimEnd('\')) {
        try { Remove-MpPreference -ExclusionPath $flDir -ErrorAction Stop } catch {}
    }
    foreach ($p in @($exeDir)) {
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

    # 3. Strong-mode secret provisioning (HKLM, ACL: SYSTEM + Administrators only).
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
        # The existing config, if any, is signed with the reduced-mode (HKCU)
        # secret. This marker lets the enforcer adopt and re-sign it once
        # instead of rejecting it as tampered (storage.Store.adoptLegacy).
        New-ItemProperty -Path $base -Name 'MigratePending' -PropertyType DWord -Value 1 -Force | Out-Null
        $bytes = New-Object byte[] 32
        [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
        $hex = ($bytes | ForEach-Object { $_.ToString('x2') }) -join ''
        New-ItemProperty -Path $secKey -Name 'SecretKey' -PropertyType String -Value $hex -Force | Out-Null
        Write-Host "[setup] Generated tamper-protection secret (strong mode)." -ForegroundColor Green
    } else {
        Write-Host "[setup] Existing tamper-protection secret preserved." -ForegroundColor Green
    }

    # 3b. Admin-only folder for the enforcer's secure config copy. Same ACL as
    #     the Security key: protected (nothing inherited from ProgramData, which
    #     lets Users create files), SYSTEM + Administrators full control, owner
    #     Administrators. The enforcer re-applies this ACL itself on start.
    #     A folder someone else created first is replaced: its contents could
    #     still be open or writable for them.
    if (Test-Path $secureDir) {
        $owner = (Get-Acl -Path $secureDir).GetOwner([Security.Principal.SecurityIdentifier])
        if ($owner -ne $adminsSid -and $owner -ne $systemSid) { Remove-Item -Path $secureDir -Recurse -Force }
    }
    if (-not (Test-Path $secureDir)) { New-Item -ItemType Directory -Path $secureDir -Force | Out-Null }
    $dirAcl = New-Object System.Security.AccessControl.DirectorySecurity
    $dirAcl.SetAccessRuleProtection($true, $false)
    $dirAcl.SetOwner($adminsSid)
    foreach ($sid in @($adminsSid, $systemSid)) {
        $dirAcl.AddAccessRule((New-Object System.Security.AccessControl.FileSystemAccessRule($sid, 'FullControl', $inherit, $prop, $allow)))
    }
    Set-Acl -Path $secureDir -AclObject $dirAcl
    Write-Host "[setup] Secured $secureDir (SYSTEM + Administrators only)." -ForegroundColor Green

    # 4. Register the enforcement task (must match taskXMLTemplate in
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
    <TimeTrigger>
      <Repetition>
        <Interval>PT1M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
      <StartBoundary>2026-01-01T00:00:00</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
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
    # Registered from memory: the temp folder is user-writable, so a file there
    # could be changed before Task Scheduler reads it.
    # The task can fire as soon as it exists, so strong mode is marked provisioned
    # first, and unmarked again if there will be no enforcer to sign for it.
    $wasProvisioned = (Get-ItemProperty -Path $base -Name 'Provisioned' -ErrorAction SilentlyContinue).Provisioned -eq 1
    New-ItemProperty -Path $base -Name 'Provisioned' -PropertyType DWord -Value 1 -Force | Out-Null
    try {
        $null = Register-ScheduledTask -TaskName $taskName -Xml $xml -Force
    } catch {
        if (-not $wasProvisioned) { Remove-ItemProperty -Path $base -Name 'Provisioned' -ErrorAction SilentlyContinue }
        throw
    }
    Write-Host "[setup] Registered '$taskName' -> `"$ExePath`" --enforce (battery-safe, 1-min auto-relaunch)." -ForegroundColor Green

    # 4b. The on-demand updater task (ADR 0009): no trigger, highest run level,
    #     runs the installed exe with --apply-update, which fetches, verifies
    #     (signed checksums, newer version only) and installs the latest
    #     release without a UAC prompt. Its security descriptor lets the
    #     signed-in user read and run it but only SYSTEM/Administrators change
    #     it, so a non-elevated process can't point it at another program.
    #     Only defined here (never by the app; dev builds don't update).
    #     Best-effort: without it, in-app updates show one UAC prompt instead.
    $updXml = @"
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Installs Focus Lock updates when you click Install and restart in the app.</Description>
    <SecurityDescriptor>D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FRFX;;;$sidXml)</SecurityDescriptor>
  </RegistrationInfo>
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
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <ExecutionTimeLimit>PT30M</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>"$cmdXml"</Command>
      <Arguments>--apply-update</Arguments>
    </Exec>
  </Actions>
</Task>
"@
    try {
        $null = Register-ScheduledTask -TaskName $updaterTaskName -Xml $updXml -Force
        Write-Host "[setup] Registered '$updaterTaskName' -> `"$ExePath`" --apply-update (updates without a UAC prompt)." -ForegroundColor Green
    } catch {
        Write-Warning "Could not register '$updaterTaskName' ($($_.Exception.Message)). In-app updates will ask for administrator approval instead."
    }

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
