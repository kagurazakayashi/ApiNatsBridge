#!/bin/bash
# ApiNatsBridge Service Launcher for Linux

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo ""
echo "============================================"
echo "  ApiNatsBridge Service Launcher"
echo "============================================"
echo ""
echo "  This script will start:"
echo "    (1) NATS Server"
echo "    (2) ApiNatsBridge"
echo "    (3) ApiNatsBridgeTemplate"
echo ""
echo "  Run serve_stop.sh to stop all services."
echo "============================================"
echo ""

mkdir -p logs

echo "*** Starting NATS Server ***"
cd test/nats-server
./nats-server -c nats-server.conf > "$SCRIPT_DIR/logs/nats-server.log" 2>&1 &
echo $! > "$SCRIPT_DIR/.nats-server.pid"
cd "$SCRIPT_DIR"
sleep 3

echo "*** Starting ApiNatsBridge ***"
go mod tidy
go generate .
go build -o ApiNatsBridge -gcflags="all=-N -l" .
./ApiNatsBridge -c test/ApiNatsBridgeConfig.yaml > "$SCRIPT_DIR/logs/apinatsbridge.log" 2>&1 &
echo $! > "$SCRIPT_DIR/.apinatsbridge.pid"
sleep 5

echo "*** Starting ApiNatsBridgeTemplate ***"
cd ApiNatsBridgeTemplate
go mod tidy
go build -o ApiNatsBridgeTemplate -gcflags="all=-N -l" .
./ApiNatsBridgeTemplate -c config.yaml -o ../logs/ApiNatsBridgeTemplate.log > "$SCRIPT_DIR/logs/template.log" 2>&1 &
echo $! > "$SCRIPT_DIR/.template.pid"
cd "$SCRIPT_DIR"
sleep 5

echo "*** Sending test ping ***"
curl -s "http://127.0.0.1:9080/ping?timestamp=$(date +%s%3N)"
echo ""

echo ""
echo "============================================"
echo "  All services started."
echo "============================================"
echo ""
