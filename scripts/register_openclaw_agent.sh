#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${1:-$ROOT_DIR/.env}"

cd "$ROOT_DIR"
go run ./cmd/agent --mode register --env-file "$ENV_FILE"
