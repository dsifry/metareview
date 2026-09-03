#!/usr/bin/env bash
set -euo pipefail

# R5: a cross-scope stale-head ledger (the #90 case — a pr-ready PASS run whose repo-wide FINDINGS.md would
# otherwise show other-scope stale findings) no longer renders a self-contradictory index. The render
# partitions stale-HEAD blockers (any target) into a labeled "## Stale" section, so the current unresolved list
# stays consistent with the passing run, and an ADVISORY hygiene note surfaces the cruft (it never blocks).

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/metareview"
(cd "$ROOT" && go build -o "$BIN" ./cmd/metareview)

verdict() { grep -A2 '## Verdict' "$1" | tail -1 | tr -d '[:space:]'; }
prlog() { "$BIN" review pr-ready --base main --evidence "$1" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true; }

repo="$(mktemp -d)"; ev="$(mktemp)"
(
  cd "$repo"
  git init -q -b main; git config user.email t@e; git config user.name t
  printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
  git checkout -q -b work
  printf 'package p\nvar X = 1\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm change

  # Seed an OPEN blocking finding for a DIFFERENT branch at a stale HEAD (the #90 shape: the projection routes it
  # to historical/unrelated so the pr-ready run passes, but the repo-wide FINDINGS.md would still render it).
  mkdir -p .metareview
  printf '{"schemaVersion":1,"id":"mrvf-stale-001","runId":"mrv-old","scope":"pr-ready","reviewer":"security-reviewer","severity":"high","classification":"blocking","status":"open","title":"Stale other-branch blocker","fingerprint":"stale:x","target":{"type":"branch","id":"otherbranch"},"gitHead":"0000000000000000000000000000000000000000","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}\n' > .metareview/findings.jsonl

  "$BIN" evidence run -- true > "$ev" 2>/dev/null
  "$BIN" review record-lenses --scope pr-ready --base main --lenses security --verdict PASS --mode in-session-emulated >/dev/null 2>&1

  log="$(prlog "$ev")"
  got="$(verdict "$log")"
  # The run PASSES (unrelated other-branch cruft does not block); the hygiene note is ADVISORY.
  if [ "$got" = "NEEDS_REVISION" ]; then echo "FAIL: [advisory] unrelated cruft must NOT block: $got"; exit 1; fi
  grep -q 'generated-artifact-hygiene-reviewer' "$log" || { echo "FAIL: [advisory] hygiene note missing from $log"; exit 1; }

  # The render partitions the stale blocker into a "## Stale" section, NOT the current unresolved list.
  idx="docs/metareview/FINDINGS.md"
  grep -q '## Stale' "$idx" || { echo "FAIL: [render] no Stale section"; cat "$idx"; exit 1; }
  before="$(awk '/## Stale/{exit} {print}' "$idx")"
  if printf '%s' "$before" | grep -q 'mrvf-stale-001'; then echo "FAIL: [render] stale blocker in the current list"; exit 1; fi
  grep -q 'mrvf-stale-001' "$idx" || { echo "FAIL: [render] stale blocker missing entirely"; exit 1; }
  echo "ok: cross-scope-cruft-advises-and-partitions -> $got"
)
rm -rf "$repo" "$ev"

# Negative: a clean ledger (no stale-head findings) → no advisory note, no Stale section.
repo="$(mktemp -d)"; ev="$(mktemp)"
(
  cd "$repo"
  git init -q -b main; git config user.email t@e; git config user.name t
  printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
  git checkout -q -b work
  printf 'package p\nvar X = 1\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm change
  "$BIN" evidence run -- true > "$ev" 2>/dev/null
  "$BIN" review record-lenses --scope pr-ready --base main --lenses security --verdict PASS --mode in-session-emulated >/dev/null 2>&1
  log="$(prlog "$ev")"
  if grep -q 'generated-artifact-hygiene-reviewer' "$log"; then echo "FAIL: [clean] hygiene note with no stale finding"; exit 1; fi
  grep -q '## Stale' "docs/metareview/FINDINGS.md" && { echo "FAIL: [clean] Stale section with no stale finding"; exit 1; }
  echo "ok: clean-ledger-no-note"
)
rm -rf "$repo" "$ev"

echo "test-generated-artifact-hygiene: ok"
