Add-Type -AssemblyName Microsoft.VisualBasic
Add-Type -AssemblyName System.Windows.Forms

$tempFile = Join-Path $env:TEMP "focuslock_minutes.txt"
Remove-Item $tempFile -ErrorAction SilentlyContinue

$min = 1
$max = 500

while ($true) {
    $input = [Microsoft.VisualBasic.Interaction]::InputBox(
        "Enter lock duration in minutes (1-500):",
        "Focus Lock",
        ""
    )

    # Cancel or close
    if ($input -eq "") {
        exit 0
    }

    if ($input -notmatch '^\d+$') {
        [System.Windows.Forms.MessageBox]::Show(
            "Please enter a numeric value.",
            "Invalid Input",
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Error
        ) | Out-Null
        continue
    }

    $value = [int]$input

    if ($value -lt $min -or $value -gt $max) {
        [System.Windows.Forms.MessageBox]::Show(
            "Please enter a value between 1 and 500.",
            "Invalid Range",
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Error
        ) | Out-Null
        continue
    }

    Set-Content -Path $tempFile -Value $value -Encoding ASCII
    exit 0
}
