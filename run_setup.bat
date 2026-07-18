@echo off
title Windows Provisioning Tool

:: Auto-elevate to Administrator if not already elevated
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Requesting Administrator privileges...
    powershell -Command "Start-Process '%~f0' -Verb RunAs"
    exit /b
)

cd /d "%~dp0"
echo.
echo ===================================================
echo  Windows Provisioning Tool
echo ===================================================
echo.
echo Running Setup.exe...
Setup.exe
echo.
echo ===================================================
echo  Provisioning complete.
echo ===================================================
pause
