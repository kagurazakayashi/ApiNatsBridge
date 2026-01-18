@echo off
python go-build-all\build_all.py --target "windows/amd64,windows/arm64,linux/amd64,linux/arm64,darwin/amd64,darwin/arm64" --jobs 0 %*
