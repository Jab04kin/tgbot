# Установка бота как службы Windows
# Запускать от имени администратора

$serviceName = "OsteomerchBot"
$servicePath = "$PSScriptRoot\start_bot.ps1"
$serviceDescription = "Telegram bot for Osteomerch clothing recommendations"

# Создаем службу
New-Service -Name $serviceName -BinaryPathName "powershell.exe -ExecutionPolicy Bypass -File `"$servicePath`"" -DisplayName "Osteomerch Telegram Bot" -Description $serviceDescription -StartupType Automatic

Write-Host "Служба $serviceName создана успешно!"
Write-Host "Запуск службы..."
Start-Service $serviceName

Write-Host "Бот установлен как служба и запущен автоматически!"
Write-Host "Служба будет запускаться при каждом старте Windows."
