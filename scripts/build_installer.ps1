# Focus Lock - build the bundled installer
#
# Runs `wails build` to produce build\bin\focus-lock.exe, then compiles the
# Inno Setup script (installer\focus-lock.iss) into a single Setup.exe at
# installer\Output.
#
# Prereqs: Go, Node, the Wails CLI, and Inno Setup 6 (https://jrsoftware.org/,
# which provides ISCC.exe). Run from anywhere in the repo.
#
#   powershell -ExecutionPolicy Bypass -File scripts\build_installer.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\build_installer.ps1 -Version 2026.11.17.0
#
# -Version (calendar version YYYY.M.D.N, used by the release workflow) stamps
# the app (backend/version.Version, which enables in-app updates) and the
# installer. Without it this is a local "dev" build (app version "dev", which
# never offers updates; installer version 0.0.0.0).
#
# Outputs in installer\Output: FocusLock-Setup-<ver>.exe, a stable-named copy
# FocusLock-Setup.exe, and SHA256SUMS.txt ("<sha256>  <file>" per exe).

#requires -Version 5
param(
    [string]$Version = ''
)
$ErrorActionPreference = 'Stop'

if ($Version) {
    $Version = $Version.TrimStart('v')
    if ($Version -notmatch '^\d{1,5}\.\d{1,5}\.\d{1,5}\.\d{1,5}$' -or
        ($Version.Split('.') | Where-Object { [int]$_ -gt 65535 })) {
        throw "-Version must be a 4-part numeric version (YYYY.M.D.N, each part <= 65535), got '$Version'"
    }
}
$installerVersion = if ($Version) { $Version } else { '0.0.0.0' }

$repoRoot = $null
try { $repoRoot = (& git rev-parse --show-toplevel 2>$null) } catch {}
if (-not $repoRoot) { $repoRoot = Split-Path -Parent $PSScriptRoot }
Set-Location $repoRoot

# --- 1. Build the app ---
Write-Host "[installer] Building Focus Lock $installerVersion (wails build)..." -ForegroundColor Cyan
if ($Version) {
    & wails build -ldflags "-X focus-lock/backend/version.Version=$Version"
} else {
    & wails build
}
if ($LASTEXITCODE -ne 0) { throw "wails build failed (exit $LASTEXITCODE)" }

$built = Join-Path $repoRoot 'build\bin\focus-lock.exe'
if (-not (Test-Path $built)) { throw "Build output not found: $built" }

# --- 2. Locate the Inno Setup compiler (ISCC.exe) ---
$iscc = (Get-Command ISCC.exe -ErrorAction SilentlyContinue).Source
if (-not $iscc) {
    foreach ($c in @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe")) {   # per-user (winget --scope user) install
        if (Test-Path $c) { $iscc = $c; break }
    }
}
if (-not $iscc) {
    throw "ISCC.exe (Inno Setup 6) not found. Install it from https://jrsoftware.org/isdl.php, or add ISCC.exe to PATH."
}

# --- 3. Compile the installer ---
$iss = Join-Path $repoRoot 'installer\focus-lock.iss'
Write-Host "[installer] Compiling $iss ..." -ForegroundColor Cyan
& $iscc "/DAppVersion=$installerVersion" $iss
if ($LASTEXITCODE -ne 0) { throw "ISCC failed (exit $LASTEXITCODE)" }

# --- 4. Stable-named copy + checksums (release assets) ---
$outDir = Join-Path $repoRoot 'installer\Output'
$setup = Join-Path $outDir "FocusLock-Setup-$installerVersion.exe"
if (-not (Test-Path $setup)) { throw "Installer output not found: $setup" }
$stable = Join-Path $outDir 'FocusLock-Setup.exe'
Copy-Item $setup $stable -Force

$sums = foreach ($f in @($setup, $stable)) {
    $hash = (Get-FileHash -Algorithm SHA256 $f).Hash.ToLowerInvariant()
    "$hash  $(Split-Path -Leaf $f)"
}
# LF line endings, no BOM: the app's updater parses this file.
$sumsPath = Join-Path $outDir 'SHA256SUMS.txt'
[System.IO.File]::WriteAllText($sumsPath, (($sums -join "`n") + "`n"), (New-Object System.Text.UTF8Encoding $false))

Write-Host "[installer] Done:" -ForegroundColor Green
foreach ($f in @($setup, $stable, $sumsPath)) {
    Write-Host "  -> $f" -ForegroundColor Green
}
