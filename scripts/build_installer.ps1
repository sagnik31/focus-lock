# Focus Lock - build the bundled installer
#
# Runs `wails build` to produce build\bin\focus-lock.exe, then compiles the
# Inno Setup script (build\installer\focus-lock.iss) into a single Setup.exe at
# build\installer\Output.
#
# Prereqs: Go, Node, the Wails CLI, and Inno Setup 6 (https://jrsoftware.org/,
# which provides ISCC.exe). Run from anywhere in the repo.
#
#   powershell -ExecutionPolicy Bypass -File scripts\build_installer.ps1

#requires -Version 5
$ErrorActionPreference = 'Stop'

$repoRoot = (& git rev-parse --show-toplevel 2>$null)
if (-not $repoRoot) { $repoRoot = Split-Path -Parent $PSScriptRoot }
Set-Location $repoRoot

# --- 1. Build the app ---
Write-Host "[installer] Building Focus Lock (wails build)..." -ForegroundColor Cyan
& wails build
if ($LASTEXITCODE -ne 0) { throw "wails build failed (exit $LASTEXITCODE)" }

$built = Join-Path $repoRoot 'build\bin\focus-lock.exe'
if (-not (Test-Path $built)) { throw "Build output not found: $built" }

# --- 2. Locate the Inno Setup compiler (ISCC.exe) ---
$iscc = (Get-Command ISCC.exe -ErrorAction SilentlyContinue).Source
if (-not $iscc) {
    foreach ($c in @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe")) {
        if (Test-Path $c) { $iscc = $c; break }
    }
}
if (-not $iscc) {
    throw "ISCC.exe (Inno Setup 6) not found. Install it from https://jrsoftware.org/isdl.php, or add ISCC.exe to PATH."
}

# --- 3. Compile the installer ---
$iss = Join-Path $repoRoot 'installer\focus-lock.iss'
Write-Host "[installer] Compiling $iss ..." -ForegroundColor Cyan
& $iscc $iss
if ($LASTEXITCODE -ne 0) { throw "ISCC failed (exit $LASTEXITCODE)" }

$outDir = Join-Path $repoRoot 'installer\Output'
Write-Host "[installer] Done. Setup.exe is in $outDir" -ForegroundColor Green
Get-ChildItem $outDir -Filter *.exe -ErrorAction SilentlyContinue | ForEach-Object {
    Write-Host "  -> $($_.FullName)" -ForegroundColor Green
}
