#!/usr/bin/env bash
# The SessionStart notice (#194): with metareview's git gate installed, it must also say when the repository has
# NOT opted into the Stop gate — an install from before #194 loses the Stop gate on upgrade, and nothing else in
# the session would say so. With both in place it says nothing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
# The hook scripts are materialized under the user's data home (#173): keep them out of the real one.
export XDG_DATA_HOME="$TMP/xdg"
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

# metareview's OWN paths are not "another tool": install reclaims them without --force, so the notice must steer
# to --install-hooks, not claim a refusal. (1) The scripts vanished from the git-ignored materialized dir;
# (2) a legacy install pointing at hooks/git whose pre-push is metareview's.
own="$TMP/own"
mkdir -p "$own"
cd "$own"
git init -q -b main
for gone in "$XDG_DATA_HOME/metareview/git-hooks/0000000000000000" "$own/.metareview/git-hooks"; do
  git config core.hooksPath "$gone"
  out="$(CLAUDE_PROJECT_DIR="$own" bash "$HOOK")"
  if printf '%s' "$out" | grep -q "another tool"; then echo "FAIL: metareview's own vanished hook dir $gone is not another tool's: $out"; exit 1; fi
done
mkdir -p hooks/git
printf '#!/bin/sh\nexec metareview review gate --push\n' > hooks/git/pre-push
git config core.hooksPath hooks/git
out="$(CLAUDE_PROJECT_DIR="$own" bash "$HOOK")"
if printf '%s' "$out" | grep -q "another tool"; then echo "FAIL: metareview's legacy hooks/git is not another tool's: $out"; exit 1; fi

echo "test-session-start-check: ok"
