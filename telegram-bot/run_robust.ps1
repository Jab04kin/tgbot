# Robust bot runner with auto-restart
Write-Host "Starting Osteomerch bot with auto-restart..."

Set-Location $PSScriptRoot

while ($true) {
    try {
        Write-Host "Starting bot at $(Get-Date)..."
        $process = Start-Process -FilePath "go" -ArgumentList "run", "main.go" -WindowStyle Hidden -PassThru
        
        # Wait for process to complete
        $process.WaitForExit()
        
        Write-Host "Bot stopped with exit code: $($process.ExitCode)"
        Write-Host "Restarting in 5 seconds..."
        Start-Sleep -Seconds 5
        
    } catch {
        Write-Host "Error: $($_.Exception.Message)"
        Write-Host "Restarting in 10 seconds..."
        Start-Sleep -Seconds 10
    }
}
