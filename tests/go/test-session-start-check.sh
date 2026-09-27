#!/usr/bin/env bash
# The SessionStart notice (#194): with metareview's git gate installed, it must also say when the repository has
# NOT opted into the Stop gate — an install from before #194 loses the Stop gate on upgrade, and nothing else in
# the session would say so. With both in place it says nothing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
HOOK="$ROOT/hooks/session-start-check.sh"
(cd "$ROOT" && go build -o "$TMP/mrv" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo"
cd "$repo"
git init -q -b main
git config user.email t@e
git config user.name t
git -c commit.gpgsign=false commit -q --allow-empty -m base
"$TMP/mrv" setup --install-hooks --yes >/dev/null

out="$(CLAUDE_PROJECT_DIR="$repo" bash "$HOOK")"
if [ -n "$out" ]; then echo "FAIL: an installed, opted-in repository needs no notice, got: $out"; exit 1; fi

git config --local --unset metareview.stopGate
out="$(CLAUDE_PROJECT_DIR="$repo" bash "$HOOK")"
printf '%s' "$out" | python3 -c 'import json,sys; json.load(sys.stdin)' || { echo "FAIL: the notice must be valid JSON: $out"; exit 1; }
printf '%s' "$out" | grep -q "enable-stop-gate" || { echo "FAIL: a missing Stop-gate opt-in must be announced, got: $out"; exit 1; }

# A repository whose own hook manager owns core.hooksPath: --install-hooks refuses there, so the notice must name
# the standalone opt-in rather than only a command that will fail.
husky="$TMP/husky"
mkdir -p "$husky/.husky"
cd "$husky"
git init -q -b main
git config core.hooksPath .husky
out="$(CLAUDE_PROJECT_DIR="$husky" bash "$HOOK")"
printf '%s' "$out" | grep -q "enable-stop-gate" || { echo "FAIL: a repo with its own hook manager must be told about --enable-stop-gate, got: $out"; exit 1; }

echo "test-session-start-check: ok"
