#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
command -v protoc >/dev/null
command -v protoc-gen-go >/dev/null
test -x "$ROOT/ui/node_modules/.bin/protoc-gen-ts_proto"

protoc -I "$ROOT/proto" \
  --go_out="$ROOT/api/internal/api/v1/proto" \
  --go_opt=paths=source_relative \
  poker.proto

protoc -I "$ROOT/proto" \
  --plugin="protoc-gen-ts_proto=$ROOT/ui/node_modules/.bin/protoc-gen-ts_proto" \
  --ts_proto_out="$ROOT/ui/src/lib/api/proto" \
  --ts_proto_opt=snakeToCamel=false,outputServices=none,esModuleInterop=true,useOptionals=messages \
  poker.proto
