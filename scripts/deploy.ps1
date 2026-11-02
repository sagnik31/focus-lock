# Focus Lock — build & deploy
#
# Builds the app (frontend + backend) from the current working tree, copies the
# result to a stable location, and points the desktop shortcut at it so the
# shortcut always launches the newest build. Invoked automatically by the
# pre-push git hook (scripts/hooks/pre-push); can also be run by hand.
#
# The enforcer ("Ghost") is staged here but applies on the NEXT REBOOT: it runs
# elevated and denies its own termination (the anti-kill protection), so a
# running Ghost can't be force-restarted. The UI and shortcut update immediately.

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
# %APPDATA%\FocusLock, not %LOCALAPPDATA%: this folder already carries the
# Defender exclusion from the one-time setup (it holds the Ghost), so the UI exe
# isn't quarantined here. A previous %LOCALAPPDATA%\FocusLock build was removed
# by Defender within minutes because that path wasn't excluded.
$deployDir = Join-Path $env:APPDATA 'FocusLock'
$deployExe = Join-Path $deployDir 'focus-lock.exe'
New-Item -ItemType Directory -Force -Path $deployDir | Out-Null

# 1. Update the UI executable. If the app is open it holds the file; that's not
#    fatal — it will update on the next push after the app is closed.
try {
    Copy-Item $built $deployExe -Force
    Write-Host "[deploy] UI updated  -> $deployExe" -ForegroundColor Green
}
catch {
    Write-Warning "Could not replace $deployExe (is Focus Lock open?). It will update on the next push once the app is closed."
}

# 2. Stage the newest enforcer. The running Ghost holds its own exe, so rename
#    the old one aside and drop the new build in place; the next reboot runs it.
$ghostDir = Join-Path $env:APPDATA 'FocusLock\Bin'
$ghostExe = Join-Path $ghostDir 'FocusLockGhost.exe'
if (Test-Path $ghostDir) {
    try {
        if (Test-Path $ghostExe) {
            $old = "$ghostExe.old"
            Remove-Item $old -Force -ErrorAction SilentlyContinue
            Rename-Item $ghostExe $old -ErrorAction Stop
        }
        Copy-Item $built $ghostExe -Force
        Write-Host "[deploy] Enforcer staged (applies on next reboot)" -ForegroundColor Green
    }
    catch {
        Write-Warning "Could not stage the enforcer exe; it will refresh on the next reboot or when a session next starts it fresh."
    }
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
