#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
protoc -I ../proto --plugin=protoc-gen-dart=tool/protoc-gen-dart --dart_out=lib/generated poker.proto
dart format lib/generated
