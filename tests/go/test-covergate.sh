#!/usr/bin/env bash
# The covergate binary end to end: it is the Go-native coverage gate that replaces the parse+enforce
# half of tests/coverage.sh (spec docs/specs/2026-09-01-covergate-go-native.md). The unit tests in
# internal/covergate cover the logic; this drives the actual CLI so main() and the flag wiring are
# exercised (and, run under the coverage harness, counted), and so a wiring regression is caught.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

MODULE="github.com/dsifry/metareview"
(cd "$ROOT" && go build -o "$TMP/covergate" ./cmd/covergate)

# A profile with a required package at 100% and a floored package at 100%.
cat > "$TMP/profile.txt" <<EOF
mode: atomic
$MODULE/internal/fsm/kind/kind.go:1.1,2.2 4 4
$MODULE/internal/a/a.go:1.1,2.2 8 8
EOF
printf 'internal/a 80\n' > "$TMP/floor.txt"
printf '%s/internal/fsm/kind\n' "$MODULE" > "$TMP/require.txt"

# Pass case.
if ! "$TMP/covergate" --profile "$TMP/profile.txt" --floor "$TMP/floor.txt" \
	--module "$MODULE" --require-100 "$TMP/require.txt" > "$TMP/out" 2>&1; then
	echo "covergate: expected PASS, got failure:" >&2; cat "$TMP/out" >&2; exit 1
fi
grep -q "coverage gate passed" "$TMP/out" || { echo "covergate: missing pass line" >&2; cat "$TMP/out" >&2; exit 1; }

# Fail case: the required package drops below 100% (one uncovered block).
cat > "$TMP/profile-fail.txt" <<EOF
mode: atomic
$MODULE/internal/fsm/kind/kind.go:1.1,2.2 3 1
$MODULE/internal/fsm/kind/kind.go:3.1,4.2 1 0
EOF
if "$TMP/covergate" --profile "$TMP/profile-fail.txt" --floor "$TMP/floor.txt" \
	--module "$MODULE" --require-100 "$TMP/require.txt" > "$TMP/out2" 2>&1; then
	echo "covergate: expected FAIL for a sub-100% required package, but it passed" >&2; cat "$TMP/out2" >&2; exit 1
fi
grep -q "coverage gate FAILED" "$TMP/out2" || { echo "covergate: missing FAILED line" >&2; cat "$TMP/out2" >&2; exit 1; }

# Usage error: missing required flags exits 2.
set +e
"$TMP/covergate" --profile "$TMP/profile.txt" > /dev/null 2>&1
code=$?
set -e
[ "$code" -eq 2 ] || { echo "covergate: missing flags should exit 2, got $code" >&2; exit 1; }

echo "test-covergate.sh: OK"
