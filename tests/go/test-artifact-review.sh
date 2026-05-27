#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo/docs" "$repo/.beads/knowledge"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
printf "# Service Inventory\n\nExisting services.\n" > docs/SERVICE_INVENTORY.md
printf '{"id":"fact-1","fact":"Use constructor injection for services."}\n' > .beads/knowledge/gotchas.jsonl
printf "# Plan\n\nBuild a thing.\n" > docs/plan.md
git add .
git commit -qm "initial"

setup_json="$("$TMP/metareview" setup --check)"
printf '%s\n' "$setup_json" | grep -q '"mode": "standalone-full"'
printf '%s\n' "$setup_json" | grep -q '"serviceInventory": true'

status="$("$TMP/metareview" status)"
printf '%s\n' "$status" | grep -q 'metareview 0.1.2'
printf '%s\n' "$status" | grep -q 'beads: present'

context_path="$("$TMP/metareview" context build docs/plan.md)"
test -f "$repo/$context_path"
grep -q 'Use constructor injection' "$repo/$context_path"
grep -q 'Existing services' "$repo/$context_path"

review_path="$("$TMP/metareview" review artifact docs/plan.md)"
test -f "$repo/$review_path"
grep -q 'NOT_REVIEWED' "$repo/$review_path"
test -f .metareview/runs.jsonl
test -f .metareview/findings.jsonl
test -f docs/metareview/FINDINGS.md
first_run="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\\n').map(JSON.parse); console.log(lines[0].id)")"

second_review="$("$TMP/metareview" review artifact docs/plan.md --previous-run "$first_run")"
test -f "$repo/$second_review"
grep -q "Previous run: \`$first_run\`" "$repo/$second_review"
