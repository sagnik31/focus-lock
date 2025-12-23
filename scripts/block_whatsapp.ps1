Get-Process | Where-Object {
    ($_.Name -match "WhatsApp") -or
    ($_.Path -and $_.Path -match "WhatsApp")
} | ForEach-Object {
    try {
        Stop-Process -Id $_.Id -Force
    } catch {}
}
