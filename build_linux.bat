@ECHO OFF
REM Cross-compile ApiNatsBridge for Linux x86_64 (amd64)
SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64
SET OUTPUT=ApiNatsBridge
go build -o %OUTPUT% .
IF %ERRORLEVEL% EQU 0 (
    ECHO Build success: %OUTPUT%
) ELSE (
    ECHO Build failed.
)
