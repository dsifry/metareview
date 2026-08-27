#!/usr/bin/env bash
# internal/shardpack must be at exactly 100% statement coverage: the profile must
# exist, be non-empty, and contain no block with zero hits.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
PROFILE="$(mktemp)"
trap 'rm -f "$PROFILE"' EXIT

if ! go test -coverprofile="$PROFILE" ./internal/shardpack/ > /dev/null; then
  echo "shardpack coverage: go test failed" >&2
  exit 1
fi

blocks="$(awk 'NR > 1' "$PROFILE" | wc -l | tr -d ' ')"
if [ "$blocks" -eq 0 ]; then
  echo "shardpack coverage: profile is empty (no blocks measured)" >&2
  exit 1
fi

uncovered="$(awk 'NR > 1 && $3 == 0' "$PROFILE" | wc -l | tr -d ' ')"
if [ "$uncovered" -ne 0 ]; then
  echo "shardpack coverage: $uncovered uncovered block(s) of $blocks" >&2
  awk 'NR > 1 && $3 == 0 {print "  " $1}' "$PROFILE" >&2
  exit 1
fi

echo "shardpack coverage: 100% ($blocks blocks)"
