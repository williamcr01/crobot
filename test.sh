#!/usr/bin/env bash
set -euo pipefail

mkdir -p build

echo "==> Running all tests with race detection and coverage"
go test -race -count=1 -coverprofile=build/coverage.coverprofile ./...

echo ""
echo "==> Coverage summary"
go tool cover -func=build/coverage.coverprofile | tail -1

echo ""
echo "==> Building agent binary"
go build -o agent ./cmd/agent/

echo "==> All checks passed"
