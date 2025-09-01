# Запуск бота в фоне
Set-Location $PSScriptRoot
Start-Process -FilePath "go" -ArgumentList "run", "main.go" -WindowStyle Hidden -PassThru
Write-Host "Бот запущен в фоне. Проверьте Telegram."
