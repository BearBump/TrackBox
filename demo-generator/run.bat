@echo off
setlocal

cd /d "%~dp0"

if not exist ".venv\\Scripts\\python.exe" (
  echo [demo-generator] creating venv...
  python -m venv .venv || exit /b 1
)

call ".venv\\Scripts\\activate.bat" || exit /b 1

echo [demo-generator] installing deps...
python -m pip install --upgrade pip >nul || exit /b 1
python -m pip install -r requirements.txt || exit /b 1

echo [demo-generator] running main.py
echo [demo-generator] args: %*
python main.py %*


