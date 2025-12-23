@echo off
:: ------------------------------------------------
:: Auto-elevation
:: ------------------------------------------------
net session >nul 2>&1
if %errorLevel% neq 0 (
    powershell -Command "Start-Process '%~f0' -Verb RunAs"
    exit /b
)

set SCRIPT_DIR=%~dp0
set TEMP_FILE=%TEMP%\focuslock_minutes.txt
del "%TEMP_FILE%" 2>nul

:: ------------------------------------------------
:: Run GUI prompt (STA, visible)
:: ------------------------------------------------
powershell -NoProfile -STA -ExecutionPolicy Bypass ^
  -File "%SCRIPT_DIR%prompt_minutes.ps1"

:: ------------------------------------------------
:: Read result
:: ------------------------------------------------
if not exist "%TEMP_FILE%" exit /b

set /p MINUTES=<"%TEMP_FILE%"
del "%TEMP_FILE%" 2>nul

:: ------------------------------------------------
:: Start lock silently
:: ------------------------------------------------
powershell -ExecutionPolicy Bypass -WindowStyle Hidden ^
  -File "%SCRIPT_DIR%lock_whatsapp.ps1" -Minutes %MINUTES%
