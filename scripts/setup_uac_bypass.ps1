# Check for Administrator privileges
if (!([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")) {
    Write-Host "This script needs to be run as Administrator to create the Scheduled Task." -ForegroundColor Red
    Write-Host "Please right-click and 'Run as Administrator'."
    Read-Host "Press Enter to exit..."
    exit
}

$TaskName = "FocusLockLauncher"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
# Default path to the executable relative to this script
$ExePath = Join-Path $ScriptDir "..\build\bin\focus-lock.exe"
$ExePath = [System.IO.Path]::GetFullPath($ExePath)

if (!(Test-Path $ExePath)) {
    Write-Host "Executable not found at: $ExePath" -ForegroundColor Red
    $ExePath = Read-Host "Please enter the full path to focus-lock.exe"
    if (!(Test-Path $ExePath)) {
        Write-Host "File still not found. Exiting."
        exit
    }
}

Write-Host "Found executable at: $ExePath" -ForegroundColor Green

# 1. Create Scheduled Task
Write-Host "Creating Scheduled Task '$TaskName'..."
$Action = New-ScheduledTaskAction -Execute $ExePath
$Principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Highest
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit 0
$Trigger = New-ScheduledTaskTrigger -AtLogon # Optional: Run at logon? Maybe just manual for now.
# Actually, we don't need a trigger if we just want a shortcut to launch it. 
# But let's verify if we want it to run at startup too? 
# The user asked "stop the user from getting the UAC prompt every time they open the executable".
# Does not explicitly ask for startup. But usually these apps run at startup.
# For now, let's just make it run on demand via the shortcut.
# We can add a trigger if we want auto-start.

Register-ScheduledTask -TaskName $TaskName -Action $Action -Principal $Principal -Settings $Settings -Force

Write-Host "Task created successfully." -ForegroundColor Green

# 2. Create Desktop Shortcut
$DesktopPath = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = Join-Path $DesktopPath "Focus Lock.lnk"
$WScriptShell = New-Object -ComObject WScript.Shell
$Shortcut = $WScriptShell.CreateShortcut($ShortcutPath)

# The shortcut runs schtasks to run the task
$Shortcut.TargetPath = "schtasks.exe"
$Shortcut.Arguments = "/run /tn `"$TaskName`""
# Use the icon from the actual exe
$Shortcut.IconLocation = "$ExePath,0"
$Shortcut.Description = "Launch Focus Lock without UAC"
$Shortcut.Save()

Write-Host "Desktop shortcut created at: $ShortcutPath" -ForegroundColor Green
Write-Host "You can now launch Focus Lock from the desktop without a UAC prompt."
