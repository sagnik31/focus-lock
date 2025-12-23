param (
    [Parameter(Mandatory = $true)]
    [datetime]$EndTime
)

# ------------------------------------------------------------
# Paths & helpers
# ------------------------------------------------------------

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$blockScript = Join-Path $baseDir "block_whatsapp.ps1"

$logDir  = Join-Path $env:LOCALAPPDATA "FocusLock"
$logFile = Join-Path $logDir "bypass.log"

New-Item -ItemType Directory -Path $logDir -Force | Out-Null

function Write-BypassLog {
    param (
        [string]$Event,
        [string]$Details = ""
    )
    $ts = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss")
    Add-Content -Path $logFile -Value "[$ts] Event: $Event $Details"
}

# ------------------------------------------------------------
# Initial state
# ------------------------------------------------------------

$lastSeenTime = Get-Date

# Attempt to discover our own task name via registry (quietly)
$taskName = $null
try {
    $taskKey = Get-ChildItem "HKCU:\Software\Classes\CLSID" -ErrorAction SilentlyContinue |
        Where-Object {
            (Get-ItemProperty $_.PsPath -ErrorAction SilentlyContinue).EndTime -eq $EndTime.ToString("yyyy-MM-ddTHH:mm:ss")
        } |
        Select-Object -First 1

    if ($taskKey) {
        $taskName = (Get-ItemProperty $taskKey.PsPath).TaskName
    }
} catch {
    # ignore
}

# ------------------------------------------------------------
# Main watchdog loop
# ------------------------------------------------------------

try {
    while ($true) {

        $now = Get-Date

        # 1) Time rollback detection
        if ($now -lt $lastSeenTime) {
            Write-BypassLog -Event "System time moved backwards" `
                -Details "(last=$lastSeenTime now=$now)"
        }
        $lastSeenTime = $now

        # 2) End time reached → normal exit
        if ($now -ge $EndTime) {
            exit 0
        }

        # 3) Task missing before end time → bypass signal
        if ($taskName) {
            $exists = schtasks /query /tn $taskName > $null 2>&1
            if ($LASTEXITCODE -ne 0) {
                Write-BypassLog -Event "Scheduled task missing before end time" `
                    -Details "(task=$taskName end=$EndTime)"
                # Do NOT exit — keep enforcing as long as we can
                $taskName = $null
            }
        }

        # 4) Enforce block
        & $blockScript

        Start-Sleep -Seconds 1
    }
}
catch {
    # 5) Unexpected termination → bypass signal
    Write-BypassLog -Event "Watchdog terminated unexpectedly" `
        -Details "($_)"
    throw
}
