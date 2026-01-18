#!/bin/bash
# ApiNatsBridge Service Stopper for Linux

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo ""
echo "============================================"
echo "  ApiNatsBridge Service Stopper"
echo "============================================"
echo ""

echo "*** Stopping ApiNatsBridgeTemplate ***"
if [ -f "$SCRIPT_DIR/.template.pid" ]; then
    kill $(cat "$SCRIPT_DIR/.template.pid") 2>/dev/null
    rm -f "$SCRIPT_DIR/.template.pid"
fi
sleep 2

echo "*** Stopping ApiNatsBridge ***"
if [ -f "$SCRIPT_DIR/.apinatsbridge.pid" ]; then
    kill $(cat "$SCRIPT_DIR/.apinatsbridge.pid") 2>/dev/null
    rm -f "$SCRIPT_DIR/.apinatsbridge.pid"
fi
sleep 2

echo "*** Stopping NATS Server ***"
if [ -f "$SCRIPT_DIR/.nats-server.pid" ]; then
    kill $(cat "$SCRIPT_DIR/.nats-server.pid") 2>/dev/null
    rm -f "$SCRIPT_DIR/.nats-server.pid"
fi

# Fallback: clean up any remaining processes by name
pkill -f "nats-server" 2>/dev/null
rm -f "$SCRIPT_DIR/.template.pid" "$SCRIPT_DIR/.apinatsbridge.pid" "$SCRIPT_DIR/.nats-server.pid"

echo ""
echo "============================================"
echo "  All processes stopped."
echo "============================================"
echo ""
