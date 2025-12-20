@echo off
setlocal

cd /d "%~dp0"

echo [mockery] installing/updating mockery...
go install github.com/vektra/mockery/v2@latest || exit /b 1

echo [mockery] cleaning old generated mocks...
if exist "internal\\cache\\mocks\\*.go" del /q "internal\\cache\\mocks\\*.go" >nul 2>nul
if exist "internal\\services\\trackings\\mocks\\*.go" del /q "internal\\services\\trackings\\mocks\\*.go" >nul 2>nul

echo [mockery] generating mocks (go generate)...
go generate ./internal/services/trackings ./internal/cache || exit /b 1

echo [mockery] done


