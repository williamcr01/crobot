#!/usr/bin/env bash
set -euo pipefail

echo "==> Running all tests with race detection"
go test -race -count=1 -coverprofile=build/coverage.coverprofile ./...

echo ""
echo "==> Coverage summary"
go tool cover -func=build/coverage.coverprofile | tail -1
