@echo off
setlocal

cd /d "%~dp0"

if not exist ".venv\\Scripts\\python.exe" (
  echo [carrier-emulator] creating venv...
  python -m venv .venv || exit /b 1
)

call ".venv\\Scripts\\activate.bat" || exit /b 1

echo [carrier-emulator] installing deps...
python -m pip install --upgrade pip >nul || exit /b 1
python -m pip install -r requirements.txt || exit /b 1

echo [carrier-emulator] starting on http://localhost:9000
echo [carrier-emulator] args: %*
uvicorn app:app --host 0.0.0.0 --port 9000 %*


