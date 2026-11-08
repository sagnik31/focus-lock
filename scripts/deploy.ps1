# Focus Lock — build & deploy
#
# Builds the app (frontend + backend) from the current working tree, copies the
# result to a stable location, and points the desktop shortcut at it so the
# shortcut always launches the newest build. Invoked automatically by the
# pre-push git hook (scripts/hooks/pre-push); can also be run by hand.
#
# The enforcer is the same exe as the UI, run with --enforce. Updating the exe
# below updates both; the enforcer change applies on the NEXT REBOOT, because the
# running enforcer holds the old file until then. The UI and shortcut are live
# immediately.

#requires -Version 5
$ErrorActionPreference = 'Stop'

# --- Locate the repo root (works from the hook or when run directly) ---
$repoRoot = (& git rev-parse --show-toplevel 2>$null)
if (-not $repoRoot) { $repoRoot = Split-Path -Parent $PSScriptRoot }
Set-Location $repoRoot

# --- Build ---
Write-Host "[deploy] Building Focus Lock (frontend + backend)..." -ForegroundColor Cyan
& wails build
if ($LASTEXITCODE -ne 0) { throw "wails build failed (exit $LASTEXITCODE)" }

$built = Join-Path $repoRoot 'build\bin\focus-lock.exe'
if (-not (Test-Path $built)) { throw "Build output not found: $built" }

# --- Stable deploy location the shortcut points at ---
# %APPDATA%\FocusLock, not %LOCALAPPDATA%: this folder carries the Defender
# exclusion from the one-time setup, so the exe isn't quarantined here. A previous
# %LOCALAPPDATA%\FocusLock build was removed by Defender within minutes because
# that path wasn't excluded.
$deployDir = Join-Path $env:APPDATA 'FocusLock'
$deployExe = Join-Path $deployDir 'focus-lock.exe'
New-Item -ItemType Directory -Force -Path $deployDir | Out-Null

# 1. Update the UI executable. If the app is open it holds the file, so rename
#    the old one aside (a running exe can be renamed but not overwritten) and
#    drop the new build in place; the next launch picks it up. The stale .old is
#    cleaned up on the following deploy once the handle is released.
try {
    if (Test-Path $deployExe) {
        $oldUi = "$deployExe.old"
        Remove-Item $oldUi -Force -ErrorAction SilentlyContinue
        Rename-Item $deployExe $oldUi -ErrorAction Stop
    }
    Copy-Item $built $deployExe -Force
    Write-Host "[deploy] UI updated  -> $deployExe" -ForegroundColor Green
}
catch {
    Write-Warning "Could not update $deployExe; it will refresh on the next deploy."
}

# 2. Remove the stale staged-enforcer copy from older ("Ghost") builds. The
#    enforcer now runs from the UI exe updated above, so the Bin copy is dead.
$legacyBinDir = Join-Path $env:APPDATA 'FocusLock\Bin'
if (Test-Path $legacyBinDir) {
    Remove-Item $legacyBinDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "[deploy] Removed stale enforcer copy -> $legacyBinDir" -ForegroundColor Yellow
}

# 3. Create/refresh the desktop shortcut -> the stable UI path.
$desktop = [Environment]::GetFolderPath('Desktop')
$lnk = Join-Path $desktop 'Focus Lock.lnk'
$ws = New-Object -ComObject WScript.Shell
$sc = $ws.CreateShortcut($lnk)
$sc.TargetPath = $deployExe
$sc.WorkingDirectory = $deployDir
$sc.IconLocation = "$deployExe,0"
$sc.Save()
Write-Host "[deploy] Shortcut    -> $lnk" -ForegroundColor Green

# 4. Remove stale Focus Lock desktop shortcuts from older setups (e.g. one
#    pointing straight at build\bin\focus-lock.exe), keeping only the canonical
#    "Focus Lock.lnk" above, so the desktop doesn't accumulate duplicates.
Get-ChildItem -Path $desktop -Filter *.lnk -ErrorAction SilentlyContinue | ForEach-Object {
    if ($_.FullName -ieq $lnk) { return }
    try { $target = $ws.CreateShortcut($_.FullName).TargetPath } catch { return }
    if ($target -and ([System.IO.Path]::GetFileName($target) -ieq 'focus-lock.exe')) {
        Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
        Write-Host "[deploy] Removed stale shortcut -> $($_.Name)" -ForegroundColor Yellow
    }
}

Write-Host "[deploy] Done. UI is live now; enforcer changes apply after a reboot." -ForegroundColor Cyan
