#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

PATH="/tmp/go3/bin:$PATH" protoc \
  -I proto \
  --go_out=paths=source_relative:internal/gen \
  --go-grpc_out=paths=source_relative:internal/gen \
  proto/cpay/v1/auth.proto \
  proto/cpay/v1/payment_link.proto \
  proto/cpay/v1/checkout.proto
