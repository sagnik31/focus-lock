# scripts/get_apps.ps1
# Fetches installed applications and their icons (Base64 encoded)

Add-Type -AssemblyName System.Drawing

function Get-IconBase64 {
    param([string]$Path)
    try {
        if (-not (Test-Path $Path)) { return $null }
        $icon = [System.Drawing.Icon]::ExtractAssociatedIcon($Path)
        if ($null -eq $icon) { return $null }
        
        $bitmap = $icon.ToBitmap()
        $stream = New-Object System.IO.MemoryStream
        $bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
        $bytes = $stream.ToArray()
        $base64 = [Convert]::ToBase64String($bytes)
        
        $stream.Dispose()
        $bitmap.Dispose()
        $icon.Dispose()
        
        return "data:image/png;base64,$base64"
    } catch {
        return $null
    }
}

$apps = @()
$seen = @{}

# Registry paths for installed apps
$code = @("HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall", "HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall", "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall")

foreach ($path in $code) {
    if (Test-Path $path) {
        Get-ChildItem $path -ErrorAction SilentlyContinue | ForEach-Object {
            try {
                $props = Get-ItemProperty $_.PsPath
                $name = $props.DisplayName
                $iconPath = $props.DisplayIcon
                $installLoc = $props.InstallLocation
                
                # Filter out things without names or system components (often hidden)
                if (-not [string]::IsNullOrWhiteSpace($name) -and ($props.SystemComponent -ne 1)) {
                    
                    # Deduplicate by name
                    if (-not $seen.ContainsKey($name)) {
                        $seen[$name] = $true
                        
                        $exePath = ""
                        $finalIconPath = ""

                        # 1. Try DisplayIcon
                        if ($iconPath) {
                            # Clean up icon path (e.g. "C:\Path\To\Exe.exe,0")
                            $finalIconPath = $iconPath.Split(',')[0].Trim('"')
                            if ($finalIconPath -match "\.exe$") {
                                $exePath = $finalIconPath
                            }
                        }
                        
                        # 2. If no valid exe path yet, look in InstallLocation
                        if (-not $exePath -and $installLoc -and (Test-Path $installLoc)) {
                            # Naive heuristic: look for an .exe with similar name or largest .exe
                            $candidates = Get-ChildItem -Path $installLoc -Filter *.exe -Recurse -Depth 1 -ErrorAction SilentlyContinue
                            if ($candidates) {
                                # Pick the one that matches the app name best, or just the first one as fallback
                                $best = $candidates | Sort-Object Length -Descending | Select-Object -First 1
                                if ($best) {
                                    $exePath = $best.FullName
                                    if (-not $finalIconPath) { $finalIconPath = $exePath }
                                }
                            }
                        }

                        # Only add if we found a likely executable path (needed for icon and blocking)
                        if ($exePath -and (Test-Path $exePath)) {
                             $iconBase64 = Get-IconBase64 -Path $finalIconPath
                             # Fallback to exe icon if specific icon path failed
                             if (-not $iconBase64) {
                                 $iconBase64 = Get-IconBase64 -Path $exePath
                             }

                             # Just use the exe name for blocking (e.g. "chrome.exe")
                             $exeName = Split-Path -Leaf $exePath

                             $apps += [PSCustomObject]@{
                                 name = $name
                                 icon = $iconBase64
                                 exe  = $exeName
                             }
                        }
                    }
                }
            } catch {
                # Ignore errors for individual items
            }
        }
    }
}

# ---------------------------------------------------------
# NEW: Add Running Processes (catches portable & UWP apps that are open)
# ---------------------------------------------------------
Get-Process | Where-Object { $_.MainWindowTitle -and $_.Path } | ForEach-Object {
    try {
        $name = $_.MainWindowTitle
        # Clean up name (remove " - Google Chrome" etc if possible, but keep simple for now)
        # Or better: Use FileDescription if available
        if ($_.MainModule.FileVersionInfo.FileDescription) {
            $name = $_.MainModule.FileVersionInfo.FileDescription
        }

        $exePath = $_.Path
        $exeName = Split-Path -Leaf $exePath
        
        # Deduplicate by Exe Name (preferred) or Name
        # We use a composite key for seen check to allow same app with diff names? 
        # No, let's just use ExeName as the primary key for "seen" running apps if we want to avoid duplicates from the registry list
        
        # Check if we already have this exe in our list? 
        # The registry list logic keyed off "DisplayName". The ExeName might be different.
        # Let's check if we have seen this 'exe' already.
        
        $alreadyHave = $false
        foreach ($existing in $apps) {
            if ($existing.exe -eq $exeName) { 
                $alreadyHave = $true 
                break 
            }
        }

        if (-not $alreadyHave) {
             # Get Icon
             $iconBase64 = Get-IconBase64 -Path $exePath

             $apps += [PSCustomObject]@{
                 name = $name + " (Running)" # Distinctive, or just $name? Let's use name.
                 icon = $iconBase64
                 exe  = $exeName
             }
        }

    } catch {}
}

$apps | Sort-Object -Property name | ConvertTo-Json -Depth 2
