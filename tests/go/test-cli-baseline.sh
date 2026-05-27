#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

go test ./...

version="$(go run ./cmd/metareview --version)"
test "$version" = "0.1.2"

help="$(go run ./cmd/metareview --help)"
printf '%s\n' "$help" | grep -q 'metareview setup --check'
printf '%s\n' "$help" | grep -q 'metareview context build <path>'
printf '%s\n' "$help" | grep -q 'metareview context diff'
printf '%s\n' "$help" | grep -q 'metareview review artifact <path>'
printf '%s\n' "$help" | grep -q 'metareview review task-done'
printf '%s\n' "$help" | grep -q 'metareview review epic-ready'
printf '%s\n' "$help" | grep -q 'metareview review pr-ready'
