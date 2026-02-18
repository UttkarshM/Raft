@echo off
REM Raft + Gateway Cluster Startup Script for Windows
REM Starts 3 Raft nodes with HTTP gateways for Visual-Automation integration

echo ========================================
echo Starting Raft + Gateway Cluster
echo ========================================
echo.

REM Build the gateway binary
echo Building gateway binary...
cd /d "%~dp0.."
go build -o bin\gateway.exe cmd\gateway\main.go

if errorlevel 1 (
    echo Build failed! Exiting.
    exit /b 1
)

echo Build successful!
echo.

REM Create logs directory if it doesn't exist
if not exist logs mkdir logs

REM Start Node 1
echo Starting Node 1 (Raft: 5001, HTTP: 8001)...
start "Raft Node 1" /MIN cmd /c "bin\gateway.exe -id=node1 -raft-port=5001 -http-port=8001 > logs\node1.log 2>&1"
echo Node 1 started
timeout /t 2 >nul

REM Start Node 2
echo Starting Node 2 (Raft: 5002, HTTP: 8002)...
start "Raft Node 2" /MIN cmd /c "bin\gateway.exe -id=node2 -raft-port=5002 -http-port=8002 > logs\node2.log 2>&1"
echo Node 2 started
timeout /t 2 >nul

REM Start Node 3
echo Starting Node 3 (Raft: 5003, HTTP: 8003)...
start "Raft Node 3" /MIN cmd /c "bin\gateway.exe -id=node3 -raft-port=5003 -http-port=8003 > logs\node3.log 2>&1"
echo Node 3 started
timeout /t 2 >nul

echo.
echo ========================================
echo Cluster Started Successfully!
echo ========================================
echo.
echo Nodes:
echo   Node 1: Raft=localhost:5001 ^| HTTP=localhost:8001
echo   Node 2: Raft=localhost:5002 ^| HTTP=localhost:8002
echo   Node 3: Raft=localhost:5003 ^| HTTP=localhost:8003
echo.
echo Logs:
echo   type logs\node1.log
echo   type logs\node2.log
echo   type logs\node3.log
echo.
echo Health Check:
echo   curl http://localhost:8001/health
echo.
echo Cluster Status:
echo   curl http://localhost:8001/api/cluster/status
echo.
echo Stop Cluster:
echo   taskkill /FI "WINDOWTITLE eq Raft Node*"
echo.
echo ========================================
pause
