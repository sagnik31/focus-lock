# Silent Task Registration Script for Installer
# This script is intended to be run by the NSIS installer (as Admin)

$ErrorActionPreference = "Stop"

$TaskName = "FocusLockLauncher"
$ScriptDir = $PSScriptRoot
$ExePath = Join-Path $ScriptDir "focus-lock.exe"

# If running from installer, exe should be in the same dir
if (!(Test-Path $ExePath)) {
    # Fallback/Debug: check current dir
    $ExePath = Join-Path (Get-Location) "focus-lock.exe"
    if (!(Test-Path $ExePath)) {
        Write-Error "Could not find focus-lock.exe"
        exit 1
    }
}

$ExePath = [System.IO.Path]::GetFullPath($ExePath)

# Create Scheduled Task
$Action = New-ScheduledTaskAction -Execute $ExePath
$Principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Highest
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit 0

# Register (Force overwrite if exists)
Register-ScheduledTask -TaskName $TaskName -Action $Action -Principal $Principal -Settings $Settings -Force

exit 0
