#!/usr/bin/env bash
set -euo pipefail

# Local build helper — mirrors the Makefile build target.
# Usage: ./scripts/build.sh [output-name]
BINARY="${1:-aibodyguard}"
go build -o "$BINARY" ./cmd/aibodyguard
echo "Built: $BINARY"
