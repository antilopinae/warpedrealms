@echo off
chcp 65001 >nul
cd /d "%~dp0"

if not exist "client.exe" (
    echo [ERROR] client.exe not found.
    echo Make sure client.exe is in the same folder as this bat file.
    pause
    exit /b 1
)

if not exist "server.exe" (
    echo [ERROR] server.exe not found.
    echo Make sure server.exe is in the same folder as this bat file.
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

echo Starting server...
start "" server.exe

echo Starting client...
client.exe
set EXIT_CODE=%ERRORLEVEL%

echo Stopping server...
taskkill /im server.exe /f >nul 2>&1

echo.
if %EXIT_CODE% neq 0 (
    echo [ERROR] client.exe exited with code %EXIT_CODE%
)

pause
