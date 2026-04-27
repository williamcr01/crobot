#!/usr/bin/env bash
set -euo pipefail

BUILD_DIR="./build"

echo "==> Cleaning $BUILD_DIR"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

echo "==> Building crobot"
go build -o "$BUILD_DIR/agent" ./cmd/agent

echo "==> Done: $BUILD_DIR/agent"
