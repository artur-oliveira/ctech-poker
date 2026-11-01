#!/usr/bin/env bash
# Strips the legacy avatar_reporters set from every player profile (repeatable; see the command's doc).
#   scripts/backfill-avatar-reporters.sh ENV            counts only (dry run)
#   scripts/backfill-avatar-reporters.sh ENV -apply     writes
# Needs the operator's AWS profile: it Scans <ENV>_poker_player_profiles, which the service role cannot.
set -uo pipefail
env=${1:?usage: backfill-avatar-reporters.sh ENV [-apply]}
export AWS_PROFILE=${AWS_PROFILE:-ctech}
export AWS_REGION=${AWS_REGION:-$(aws configure get region)}
export ENVIRONMENT="$env"
cd "$(dirname "$0")/../api"
go run ./cmd/backfill-avatar-reporters ${2:-}
