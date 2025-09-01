# Run bot in background
Write-Host "Starting Osteomerch bot in background..."

# Change to script directory
Set-Location $PSScriptRoot

# Start bot in background
$job = Start-Job -ScriptBlock {
    Set-Location $using:PWD
    go run main.go
}

Write-Host "Bot started in background with Job ID: $($job.Id)"
Write-Host "To view logs: Get-Job -Id $($job.Id) | Receive-Job"
Write-Host "To stop bot: Stop-Job -Id $($job.Id)"
Write-Host ""
Write-Host "Bot will work even after closing this window!"
