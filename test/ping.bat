@echo off
for /f %%i in ('powershell -Command "[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()"') do set "ts=%%i"
set "request=POST"
set "url=http://127.0.0.1:9080/ping"
set "header=X-Timestamp-Ms: %ts%"
echo %request%: %url%
echo header: %header%
curl --verbose --request %request% --header "%header%" %url%
