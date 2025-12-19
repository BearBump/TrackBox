@echo off
setlocal

cd /d "%~dp0"
if not "%~1"=="" (
  call demo-generator\\run.bat %*
  exit /b %errorlevel%
)

set "API_BASE=http://localhost:8080"
set "EMULATOR_BASE=http://localhost:9000"
set "CARRIERS=CDEK,POST_RU"
set "COUNT=1000"
set "BATCH=200"
set "RPS=20"

echo.
echo ===== TrackBox demo-generator =====
echo Enter = default value in [brackets]
echo.

set /p "IN_COUNT=COUNT [%COUNT%]: "
if not "%IN_COUNT%"=="" set "COUNT=%IN_COUNT%"

set /p "IN_BATCH=BATCH [%BATCH%]: "
if not "%IN_BATCH%"=="" set "BATCH=%IN_BATCH%"

set /p "IN_RPS=RPS [%RPS%]: "
if not "%IN_RPS%"=="" set "RPS=%IN_RPS%"

set /p "IN_CARRIERS=CARRIERS [%CARRIERS%]: "
if not "%IN_CARRIERS%"=="" set "CARRIERS=%IN_CARRIERS%"

set /p "IN_API=API_BASE [%API_BASE%]: "
if not "%IN_API%"=="" set "API_BASE=%IN_API%"

set /p "IN_EMU=EMULATOR_BASE [%EMULATOR_BASE%]: "
if not "%IN_EMU%"=="" set "EMULATOR_BASE=%IN_EMU%"

echo.
echo Running:
echo   api-base=%API_BASE%
echo   emulator-base=%EMULATOR_BASE%
echo   count=%COUNT% batch=%BATCH% rps=%RPS% carriers=%CARRIERS%
echo.

call demo-generator\\run.bat --api-base "%API_BASE%" --emulator-base "%EMULATOR_BASE%" --count %COUNT% --batch %BATCH% --rps %RPS% --carriers "%CARRIERS%"
exit /b %errorlevel%


