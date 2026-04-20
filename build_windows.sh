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

echo "==> Copying data folder ..."
cp -r data "$OUTPUT_DIR/data"

echo "==> Copying gamedata folder ..."
cp -r gamedata "$OUTPUT_DIR/gamedata"

echo "==> Copying play.bat ..."
cp play.bat "$OUTPUT_DIR/play.bat"

echo ""
echo "Done! Send the contents of '$OUTPUT_DIR/' to your friend:"
ls -lh "$OUTPUT_DIR"
