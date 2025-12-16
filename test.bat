@ECHO OFF
CHCP 65001 >NUL
CD /D "%~dp0"

ECHO.
ECHO ============================================
ECHO   ApiNatsBridge Integration Test
ECHO ============================================
ECHO.
ECHO   This script will start:
ECHO     (1) NATS Server
ECHO     (2) Mock Microservice
ECHO     (3) ApiNatsBridge Main
ECHO.
ECHO   Close each window manually after testing.
ECHO ============================================
ECHO.

ECHO *** Starting NATS Server ***
START "NATS_Server" /D "test\nats-server\" nats-server.exe -c nats-server.conf
TIMEOUT /T 3 /NOBREAK >NUL

ECHO *** Starting Mock Microservice ***
START "Mock_Service" go run ./test/mock-microservice/ -c test\ApiNatsBridgeConfig.yaml
TIMEOUT /T 5 /NOBREAK >NUL

ECHO *** Starting ApiNatsBridge ***
START "ApiNatsBridge" go run . -c test\ApiNatsBridgeConfig.yaml
TIMEOUT /T 5 /NOBREAK >NUL

exit

ECHO.
ECHO *** Running HTTP Tests ***
ECHO.
ECHO --- test\ping.bat ---
CALL test\ping.bat
ECHO.
ECHO --- test\test.bat ---
CALL test\test.bat
ECHO.
ECHO --- test\test_form.bat ---
CALL test\test_form.bat
ECHO.

ECHO ============================================
ECHO   Test script finished.
ECHO ============================================
ECHO.
