@echo off
set "request=POST"
set "url=http://127.0.0.1:9080/test_form"
set "header=Content-Type: application/x-www-form-urlencoded"
set "data=message=hello"
echo %request%: %url%
echo header: %header%
echo body: %data%
curl --verbose --request %request% --header "%header%" --data "%data%" %url%
