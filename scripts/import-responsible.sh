#!/usr/bin/env bash
# Imports the responsible-gambling state from ctech-wallet's users table into Poker (once, repeatable).
#   scripts/import-responsible.sh ENV --dry-run     counts only
#   scripts/import-responsible.sh ENV               writes
# Needs the operator's AWS profile: it reads <ENV>_wallet_users and writes <ENV>_poker_responsible.
set -uo pipefail
env=${1:?usage: import-responsible.sh ENV [--dry-run]}
export AWS_PROFILE=${AWS_PROFILE:-ctech}
export AWS_REGION=${AWS_REGION:-$(aws configure get region)}
export ENVIRONMENT="$env"
cd "$(dirname "$0")/../api"
go run ./cmd/import-responsible ${2:-}
