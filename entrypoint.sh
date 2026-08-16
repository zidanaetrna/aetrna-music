#!/bin/bash
set -e

echo "[INFO] Starting Aetrna's Music Unified Container Supervisor..."

# Process termination trap handler
cleanup() {
    echo "[INFO] Shutdown signal received. Terminating processes..."
    kill -TERM "$NODE_PID" "$GO_PID" 2>/dev/null || true
    wait "$NODE_PID" "$GO_PID" 2>/dev/null || true
    echo "[INFO] Shutdown complete."
    exit 0
}

trap cleanup SIGTERM SIGINT EXIT

# 1. Start Node.js Voice Server
echo "[INFO] Starting Node.js Voice Server on port 3005..."
cd /app/voice-server
node server.js &
NODE_PID=$!

# Wait briefly for Voice Server Gateway connection
sleep 2

# Check if Node.js server is alive
if ! kill -0 "$NODE_PID" 2>/dev/null; then
    echo "[ERROR] Node.js Voice Server failed to start."
    exit 1
fi

# 2. Start Go Bot Backend Microservice
echo "[INFO] Starting Go Bot Microservice..."
cd /app
./aetrna-bot &
GO_PID=$!

# Monitor both processes in foreground
wait -n "$NODE_PID" "$GO_PID"

echo "[WARN] One of the core services exited unexpectedly. Shutting down container..."
exit 1
