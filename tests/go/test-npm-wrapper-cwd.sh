#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pkg="$TMP/package"
repo="$TMP/repo"
mkdir -p "$pkg/bin" "$pkg/cli" "$repo/.git"

(cd "$ROOT" && go build -o "$pkg/bin/metareview" ./cmd/metareview)
cp "$ROOT/cli/metareview.js" "$pkg/cli/metareview.js"

status="$(cd "$repo" && node "$pkg/cli/metareview.js" status)"
printf '%s\n' "$status" | grep -q 'mode: standalone-minimal'
printf '%s\n' "$status" | grep -q 'git: present'
