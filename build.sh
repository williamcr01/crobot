#!/usr/bin/env bash
set -euo pipefail

BUILD_DIR="./build"

echo "==> Cleaning $BUILD_DIR"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

echo "==> Building crobot"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build \
	-ldflags "-X crobot/internal/version.Version=$VERSION -X crobot/internal/version.Commit=$COMMIT -X crobot/internal/version.BuildDate=$BUILD_DATE" \
	-o "$BUILD_DIR/crobot" \
	./cmd/agent

echo "==> Done: $BUILD_DIR/crobot"
