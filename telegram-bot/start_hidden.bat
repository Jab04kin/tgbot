@echo off
cd /d "%~dp0"
start /min cmd /c "go run main.go > bot.log 2>&1"
echo Bot started in background. Check bot.log for output.
pause
