@echo off
chcp 65001 >nul
cd /d "%~dp0"

if not exist "client.exe" (
    echo [ERROR] client.exe not found.
    echo Make sure client.exe is in the same folder as this bat file.
    pause
    exit /b 1
)

if not exist "data\content\assets_manifest.json" (
    echo [ERROR] data folder is missing or incomplete.
    echo Expected: data\content\assets_manifest.json
    pause
    exit /b 1
)

if not exist "gamedata" (
    echo [ERROR] gamedata folder is missing.
    echo Make sure gamedata\ is in the same folder as client.exe
    pause
    exit /b 1
)

set /p SERVER="Server address (e.g. https://abc123.ngrok-free.app): "

if "%SERVER%"=="" (
    echo [ERROR] No server address entered.
    pause
    exit /b 1
)

echo Connecting to %SERVER% ...
echo.

client.exe -server "%SERVER%" 2>&1
set EXIT_CODE=%ERRORLEVEL%

echo.
if %EXIT_CODE% neq 0 (
    echo [ERROR] client.exe exited with code %EXIT_CODE%
)

pause
