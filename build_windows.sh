#!/usr/bin/env bash
set -euo pipefail

# Cross-compile WarpedRealms client for Windows (amd64).
# Usage: ./build_windows.sh [output_dir]
#
# Requirements on macOS/Linux:
#   brew install mingw-w64        # C cross-compiler for CGo
#   (Go toolchain already installed)

OUTPUT_DIR="${1:-dist/windows}"
mkdir -p "$OUTPUT_DIR"

echo "==> Building client.exe ..."
GOOS=windows \
GOARCH=amd64 \
CGO_ENABLED=1 \
CC=x86_64-w64-mingw32-gcc \
go build -v \
    -ldflags="-s -w" \
    -o "$OUTPUT_DIR/client.exe" \
    ./cmd/client

echo "==> Building server.exe ..."
GOOS=windows \
GOARCH=amd64 \
CGO_ENABLED=1 \
CC=x86_64-w64-mingw32-gcc \
go build -v \
    -ldflags="-s -w" \
    -o "$OUTPUT_DIR/server.exe" \
    ./cmd/server

echo "==> Copying data folder ..."
cp -r data "$OUTPUT_DIR/data"

echo "==> Copying gamedata folder ..."
cp -r gamedata "$OUTPUT_DIR/gamedata"

echo "==> Copying play.bat ..."
cp play.bat "$OUTPUT_DIR/play.bat"

echo "==> Copying test.bat ..."
cp test.bat "$OUTPUT_DIR/test.bat"

echo ""
echo "Done! The contents in '$OUTPUT_DIR/'"
ls -lh "$OUTPUT_DIR"
