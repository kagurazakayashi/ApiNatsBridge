@ECHO OFF
CHCP 65001 >NUL
CD /D "%~dp0"

ECHO.
ECHO ============================================
ECHO   ApiNatsBridge Service Launcher
ECHO ============================================
ECHO.
ECHO   This script will start:
ECHO     (1) NATS Server
ECHO     (2) ApiNatsBridge
ECHO     (3) ApiNatsBridgeTemplate
ECHO.
ECHO   Close each window manually after use.
ECHO ============================================
ECHO.

ECHO *** Starting NATS Server ***
CD test\nats-server
START "NATS_Server" nats-server.exe -c nats-server.conf
CD ..\..
TIMEOUT /T 3 /NOBREAK >NUL

ECHO *** Starting ApiNatsBridge ***
go mod tidy
go generate .
go build -o ApiNatsBridge.exe -gcflags="all=-N -l" .
START "ApiNatsBridge" ApiNatsBridge.exe -c test/ApiNatsBridgeConfig.yaml
TIMEOUT /T 5 /NOBREAK >NUL

ECHO *** Starting ApiNatsBridgeTemplate ***
CD ApiNatsBridgeTemplate
go mod tidy
go build -o ApiNatsBridgeTemplate.exe -gcflags="all=-N -l" .
START "ApiNatsBridgeTemplate" ApiNatsBridgeTemplate.exe -c config.yaml -o ../logs/ApiNatsBridgeTemplate.log
CD ..
TIMEOUT /T 5 /NOBREAK >NUL

ECHO *** Sending test ping ***
powershell -NoProfile -Command "$VerbosePreference='Continue'; Invoke-RestMethod -Verbose ('http://127.0.0.1:9080/ping?timestamp=' + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())"
ECHO.

ECHO ============================================
ECHO   All services started.
ECHO ============================================
ECHO.
