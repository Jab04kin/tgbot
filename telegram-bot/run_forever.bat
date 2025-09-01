@echo off
cd /d "%~dp0"
:loop
echo Starting bot...
go run main.go
echo Bot stopped. Restarting in 5 seconds...
timeout /t 5 /nobreak >nul
goto loop
