#!/usr/bin/env bash
set -euo pipefail

KVS_ARN="${1:?usage: publish-routes.sh <kvs-arn> <export-dir>}"
EXPORT_DIR="${2:?usage: publish-routes.sh <kvs-arn> <export-dir>}"
BATCH_SIZE=50

kvs() { aws cloudfront-keyvaluestore --region us-east-1 "$@"; }
etag() { kvs describe-key-value-store --kvs-arn "$KVS_ARN" --query ETag --output text; }

desired=$(find "$EXPORT_DIR" -name '*.html' -printf '%P\n' \
  | sed -e 's|\.html$||' -e 's|/index$||' \
  | grep -vx 'index' | sed 's|^|/|' | sort -u)
[ -n "$desired" ] || { echo "No exported routes found; refusing to wipe route store." >&2; exit 1; }

current=""
next_token=""
while :; do
  if [ -n "$next_token" ]; then
    page=$(kvs list-keys --kvs-arn "$KVS_ARN" --max-results "$BATCH_SIZE" --next-token "$next_token")
  else
    page=$(kvs list-keys --kvs-arn "$KVS_ARN" --max-results "$BATCH_SIZE")
  fi
  current+=$(echo "$page" | jq -r '.Items[].Key')$'\n'
  next_token=$(echo "$page" | jq -r '.NextToken // empty')
  [ -n "$next_token" ] || break
done
current=$(echo "$current" | sed '/^$/d' | sort -u)

puts=$(comm -23 <(echo "$desired") <(echo "$current"))
deletes=$(comm -13 <(echo "$desired") <(echo "$current"))

apply_batch() {
  local flag="$1" entries="$2"
  [ -n "$entries" ] || return 0
  # Each generated entry is one AWS CLI shorthand argument.
  # shellcheck disable=SC2086
  kvs update-keys --kvs-arn "$KVS_ARN" --if-match "$(etag)" "$flag" $entries >/dev/null
}

echo "$puts" | sed '/^$/d' | xargs -r -n "$BATCH_SIZE" | while read -r batch; do
  args=""; for route in $batch; do args+="Key=${route},Value=1 "; done
  apply_batch --puts "$args"
done
echo "$deletes" | sed '/^$/d' | xargs -r -n "$BATCH_SIZE" | while read -r batch; do
  args=""; for route in $batch; do args+="Key=${route} "; done
  apply_batch --deletes "$args"
done
