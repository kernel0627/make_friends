@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\status-all.ps1" %*
