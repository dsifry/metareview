#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo/.git" "$repo/docs"
printf "# Services\n" > "$repo/docs/SERVICE_INVENTORY.md"

(cd "$repo" && "$TMP/metareview" setup --check > "$TMP/setup.json")
grep -q '"mode": "standalone-minimal"' "$TMP/setup.json"
grep -q '"prerequisites"' "$TMP/setup.json"
grep -q '"superpowers"' "$TMP/setup.json"
grep -q '"beads"' "$TMP/setup.json"
grep -q '"metaswarm"' "$TMP/setup.json"
grep -q '"go"' "$TMP/setup.json"
grep -q '"git"' "$TMP/setup.json"
grep -q '"install"' "$TMP/setup.json"
grep -q '"path":' "$TMP/setup.json"
test ! -e "$repo/.metareview"

(cd "$repo" && "$TMP/metareview" setup --bootstrap-prereqs --dry-run > "$TMP/bootstrap.out")
grep -q 'Install Superpowers' "$TMP/bootstrap.out"
grep -q 'Install Beads' "$TMP/bootstrap.out"
grep -q 'Install metaswarm' "$TMP/bootstrap.out"
grep -q 'No changes made' "$TMP/bootstrap.out"
test ! -e "$repo/.metareview"

set +e
(cd "$repo" && "$TMP/metareview" setup --bootstrap-prereqs > "$TMP/bootstrap-confirm.out" 2>"$TMP/bootstrap-confirm.err")
code=$?
set -e
test "$code" -eq 2
grep -q 'requires --confirm-bootstrap-prereqs' "$TMP/bootstrap-confirm.err"
