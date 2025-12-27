# scripts/get_icons.ps1
# Extracts icons for a list of execuatble paths provided in a JSON file
# Input: Path to a JSON file containing an array of strings (paths)
# Output: JSON object { "C:\\Path\\To\\App.exe": "data:image/png;base64,..." }

param([string]$InputPath)

Add-Type -AssemblyName System.Drawing

function Get-IconBase64 {
    param([string]$Path)
    try {
        if (-not (Test-Path $Path)) { return $null }
        # ExtractAssociatedIcon works for most exe/lnk files
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

if (-not (Test-Path $InputPath)) {
    Write-Output "{}"
    exit
}

$paths = Get-Content $InputPath | ConvertFrom-Json
$result = @{}

foreach ($path in $paths) {
    if (-not [string]::IsNullOrWhiteSpace($path)) {
        # Check cache or process? No, just process.
        $icon = Get-IconBase64 -Path $path
        
        if ($icon) {
            $result[$path] = $icon
        }
    }
}

$result | ConvertTo-Json -Depth 2
