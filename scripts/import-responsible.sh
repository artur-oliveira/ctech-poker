#!/usr/bin/env bash
# Imports the responsible-gambling state from ctech-wallet into Poker (once, repeatable).
#   scripts/import-responsible.sh ENV --dry-run     counts only
#   scripts/import-responsible.sh ENV               writes
# Needs the operator's AWS profile; the client secret is read from SSM and never printed.
set -uo pipefail
env=${1:?usage: import-responsible.sh ENV [--dry-run]}
export AWS_PROFILE=${AWS_PROFILE:-ctech}
export AWS_REGION=${AWS_REGION:-$(aws configure get region)}
g() { aws ssm get-parameter --with-decryption --name "$1" --query Parameter.Value --output text; }
export ENVIRONMENT="$env"
export WALLET_URL=$(g "/ctech/$env/poker/wallet-url") || exit 1
export POKER_CLIENT_ID=$(g "/ctech/$env/poker/poker-client-id") || exit 1
export POKER_CLIENT_SECRET=$(g "/ctech/$env/poker/poker-client-secret") || exit 1
export CTECH_URL=${CTECH_URL:-https://accounts-api.aoctech.app}
cd "$(dirname "$0")/../api"
go run ./cmd/import-responsible ${2:-}
