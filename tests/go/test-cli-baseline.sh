#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

go test ./...

version="$(go run ./cmd/metareview --version)"
test "$version" = "$(node -p "require('./package.json').version")"

help="$(go run ./cmd/metareview --help)"
printf '%s\n' "$help" | grep -q 'metareview setup --check'
printf '%s\n' "$help" | grep -q 'metareview context build <path>'
printf '%s\n' "$help" | grep -q 'metareview context diff'
printf '%s\n' "$help" | grep -q 'metareview evidence run --'
printf '%s\n' "$help" | grep -q 'metareview evidence import --github-checks'
printf '%s\n' "$help" | grep -q 'metareview review artifact <path>'
printf '%s\n' "$help" | grep -q -- '--scaffold-only'
printf '%s\n' "$help" | grep -q 'metareview review task-done'
printf '%s\n' "$help" | grep -q 'metareview review epic-ready'
printf '%s\n' "$help" | grep -q 'metareview review pr-ready'

# status --json is the contract a host hook branches on: valid JSON on stdout, a `blocked`
# flag, and exit 1 when something must be cleared so the common decision needs no parsing.
# Exercised here rather than in a Go test because the exit decision lives in main's dispatch.
clean="$(mktemp -d)"
trap 'rm -rf "$clean"' EXIT
go build -o "$clean/mrv" ./cmd/metareview

out="$(cd "$clean" && ./mrv status --json)" || { echo "FAIL: status --json on a clean tree exited nonzero"; exit 1; }
printf '%s' "$out" | grep -q '"blocked": false' || { echo "FAIL: clean tree must not be blocked"; exit 1; }
printf '%s' "$out" | grep -q '"must_clear": \[\]' || { echo "FAIL: clean tree must have an empty must_clear"; exit 1; }

mkdir -p "$clean/docs/metareview/reviews"
printf '# metareview: task-done review\n\nRun ID: `mrv-x`\nTarget: `t-1`\n\n## Verdict\n\nNEEDS_REVISION\n' \
  > "$clean/docs/metareview/reviews/mrv-x-task-done-t-1.md"
if (cd "$clean" && ./mrv status --json >/dev/null); then
  echo "FAIL: status --json must exit nonzero when something must be cleared"; exit 1
fi
# the binary exits 1 by design here, and pipefail would fail the pipeline before grep runs
blocked_out="$( (cd "$clean" && ./mrv status --json 2>/dev/null) || true )"
printf '%s' "$blocked_out" | grep -q 't-1' || { echo "FAIL: the blocker must be named in the output"; exit 1; }
printf '%s' "$blocked_out" | grep -q '"blocked": true' || { echo "FAIL: blocked must be true"; exit 1; }
