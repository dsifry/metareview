#!/usr/bin/env bash
# The override mechanism must stay at 100% statement coverage: the profile must
# exist, contain override.go blocks, and have none with zero hits. (The rest of
# internal/findings is legacy and keeps its floor.)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
PROFILE="$(mktemp)"
trap 'rm -f "$PROFILE"' EXIT

if ! go test -coverprofile="$PROFILE" ./internal/findings/ > /dev/null; then
  echo "override coverage: go test failed" >&2
  exit 1
fi

blocks="$(awk 'NR > 1 && $1 ~ /internal\/findings\/override\.go/' "$PROFILE" | wc -l | tr -d ' ')"
if [ "$blocks" -eq 0 ]; then
  echo "override coverage: no override.go blocks in the profile" >&2
  exit 1
fi

uncovered="$(awk 'NR > 1 && $3 == 0 && $1 ~ /internal\/findings\/override\.go/' "$PROFILE" | wc -l | tr -d ' ')"
if [ "$uncovered" -ne 0 ]; then
  echo "override coverage: $uncovered uncovered block(s) of $blocks" >&2
  awk 'NR > 1 && $3 == 0 && $1 ~ /internal\/findings\/override\.go/ {print "  " $1}' "$PROFILE" >&2
  exit 1
fi

echo "override coverage: 100% ($blocks blocks)"
