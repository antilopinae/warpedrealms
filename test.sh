#!/usr/bin/env bash

cd "$(dirname "$0")"

if[ ! -f "./client" ]; then
    echo "[ERROR] client not found."
    echo "Make sure client is in the same folder as this script."
    read -n 1 -s -r -p "Press any key to exit..."
    exit 1
fi

if [ ! -f "./server" ]; then
    echo "[ERROR] server not found."
    echo "Make sure server is in the same folder as this script."
    read -n 1 -s -r -p "Press any key to exit..."
    exit 1
fi

if [ ! -f "data/content/assets_manifest.json" ]; then
    echo "[ERROR] data folder is missing or incomplete."
    echo "Expected: data/content/assets_manifest.json"
    read -n 1 -s -r -p "Press any key to exit..."
    exit 1
fi

if [ ! -d "gamedata" ]; then
    echo "[ERROR] gamedata folder is missing."
    echo "Make sure gamedata/ is in the same folder as client"
    read -n 1 -s -r -p "Press any key to exit..."
    exit 1
fi

echo "Starting server..."
./server &
SERVER_PID=$!

trap 'echo -e "\nStopping server..."; kill $SERVER_PID 2>/dev/null || true' EXIT

sleep 1

echo "Starting client..."
./client
EXIT_CODE=$?

echo ""
if[ $EXIT_CODE -ne 0 ]; then
    echo "[ERROR] client exited with code $EXIT_CODE"
fi

read -n 1 -s -r -p "Press any key to exit..."
echo ""