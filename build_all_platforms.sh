#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-all}"

build_platform() {
    local OS=$1
    local ARCH=$2
    local OUT_DIR="dist/$OS"
    local CC_FLAG=""
    local EXT=""
    local IS_WINDOWS=false
    local USE_DOCKER=false

    if [ "$OS" == "windows" ]; then
        CC_FLAG="CC=x86_64-w64-mingw32-gcc"
        EXT=".exe"
        IS_WINDOWS=true
    elif [ "$OS" == "darwin" ]; then
        OUT_DIR="dist/macos"
    elif [ "$OS" == "linux" ]; then
        if [ "$(uname -s)" == "Darwin" ]; then
            USE_DOCKER=true
        fi
    fi

    echo "========================================"
    echo "==> Building for $OS/$ARCH into $OUT_DIR"
    echo "========================================"

    rm -rf "$OUT_DIR"
    mkdir -p "$OUT_DIR"

    if [ "$USE_DOCKER" = true ]; then
        echo " -> [INFO] Mac to Linux cross-compilation detected."

        if ! command -v docker &> /dev/null; then
            echo " -> [ERROR] Docker is not installed or not running!"
            exit 1
        fi

        local DOCKER_CMD=(docker run --rm -v "${PWD}:/app" -w /app)
        local APT_CMD=""
        local BUILD_ENV="GOOS=linux GOARCH=$ARCH CGO_ENABLED=1"

        # Определяем архитектуру хоста (Mac)
        local HOST_ARCH
        HOST_ARCH=$(uname -m)

        if [ "$HOST_ARCH" == "arm64" ] && [ "$ARCH" == "amd64" ]; then
            echo " -> [INFO] Apple Silicon (ARM64) host detected."
            echo " -> [INFO] Using NATIVE container with Debian multiarch for stable cross-compilation (bypassing QEMU bugs)..."

            # Настраиваем multiarch: качаем библиотеки amd64 в arm64 контейнер
            APT_CMD="dpkg --add-architecture amd64 && apt-get update -qq && apt-get install -y -qq gcc-x86-64-linux-gnu pkg-config libx11-dev:amd64 libxcursor-dev:amd64 libxrandr-dev:amd64 libxinerama-dev:amd64 libxi-dev:amd64 libglx-dev:amd64 libgl1-mesa-dev:amd64 libxxf86vm-dev:amd64 libasound2-dev:amd64 >/dev/null"
            BUILD_ENV="$BUILD_ENV CC=x86_64-linux-gnu-gcc PKG_CONFIG=x86_64-linux-gnu-pkg-config"
        else
            echo " -> [INFO] Using standard Docker compilation..."
            APT_CMD="apt-get update -qq && apt-get install -y -qq gcc libc6-dev libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libglx-dev libgl1-mesa-dev libxxf86vm-dev libasound2-dev pkg-config >/dev/null"
        fi

        # Запускаем сборку внутри докера
        "${DOCKER_CMD[@]}" golang:latest bash -c "
            export DEBIAN_FRONTEND=noninteractive
            echo ' -> Installing Linux dependencies...' && \
            $APT_CMD && \
            echo ' -> Configuring Git safe directory...' && \
            git config --global --add safe.directory /app && \
            echo ' -> Compiling client in Docker...' && \
            env $BUILD_ENV go build -v -ldflags='-s -w' -o $OUT_DIR/client$EXT ./cmd/client && \
            echo ' -> Compiling server in Docker...' && \
            env $BUILD_ENV go build -v -ldflags='-s -w' -o $OUT_DIR/server$EXT ./cmd/server
        "
    else
        echo " -> Compiling client..."
        env GOOS=$OS GOARCH=$ARCH CGO_ENABLED=1 $CC_FLAG \
            go build -v -ldflags="-s -w" -o "$OUT_DIR/client$EXT" ./cmd/client

        echo " -> Compiling server..."
        env GOOS=$OS GOARCH=$ARCH CGO_ENABLED=1 $CC_FLAG \
            go build -v -ldflags="-s -w" -o "$OUT_DIR/server$EXT" ./cmd/server
    fi

    echo " -> Copying assets..."
    cp -r data "$OUT_DIR/data"
    cp -r gamedata "$OUT_DIR/gamedata"

    if [ "$IS_WINDOWS" = true ]; then
        cp play.bat "$OUT_DIR/play.bat"
        cp test.bat "$OUT_DIR/test.bat"
    else
        cp play.sh "$OUT_DIR/play.sh"
        cp test.sh "$OUT_DIR/test.sh"
        chmod +x "$OUT_DIR/play.sh" "$OUT_DIR/test.sh"
    fi

    echo "==> Done! Contents in '$OUT_DIR/'"
    ls -lh "$OUT_DIR"
    echo ""
}

case "$TARGET" in
    "windows")
        build_platform "windows" "amd64"
        ;;
    "linux")
        build_platform "linux" "amd64"
        ;;
    "macos")
        build_platform "darwin" "arm64"
        ;;
    "all")
        build_platform "windows" "amd64"
        build_platform "linux" "amd64"
        build_platform "darwin" "arm64"
        ;;
    *)
        echo "Usage: $0 [windows|linux|macos|all]"
        exit 1
        ;;
esac