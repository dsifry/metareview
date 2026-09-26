#!/usr/bin/env bash
set -euo pipefail

# A pr-ready run must not block on pr-ready's OWN earlier findings for the same branch. The
# "Unresolved review blockers" reviewer read every open ledger finding as a blocked review — including
# the one it had itself raised — so a standalone re-run (no --previous-run) at the same head raised a
# branch-wide blocker that every later run inherited under the same id and re-raised from itself.
# Only a human override could break the loop (seen on the 0.13.2 release branch, 2026-09-26). The
# current run re-runs every pr-ready reviewer, so an earlier pr-ready finding is either re-raised on
# its merits or gone; task-done/epic-ready blockers, and the escalation lock, are unaffected.

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/metareview"
(cd "$ROOT" && go build -o "$BIN" ./cmd/metareview)

verdict() { grep -A2 '## Verdict' "$1" | tail -1 | tr -d '[:space:]'; }
run_id() { basename "$1" .md; }
gate() { "$BIN" review pr-ready --base main "$@" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true; }

repo="$TMP/repo"
mkdir -p "$repo"
cd "$repo"
git init -q -b main
git config user.email t@e; git config user.name t
printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
git checkout -q -b work
printf 'package p\nvar X = 1\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm change
ev="$TMP/ev.jsonl"
"$BIN" evidence run -- true > "$ev" 2>/dev/null
"$BIN" review record-lenses --scope pr-ready --base main --lenses security --verdict PASS --mode in-session-emulated >/dev/null 2>&1

# 1. Attempt 1 without evidence: a real blocker (missing validation evidence).
first="$(gate)"
[ "$(verdict "$first")" = "NEEDS_REVISION" ] || { echo "FAIL: attempt 1 should need revision: $(verdict "$first")"; exit 1; }

# 2. The accidental standalone re-run at the same head, again without --previous-run.
second="$(gate)"
[ "$(verdict "$second")" = "NEEDS_REVISION" ] || { echo "FAIL: the re-run should still need revision"; exit 1; }

# 3. The real chain continues from attempt 1 WITH evidence. Nothing is wrong with the branch, so it
#    must pass — not block on pr-ready's own earlier findings.
third="$(gate --evidence "$ev" --previous-run "$(run_id "$first")")"
got="$(verdict "$third")"
if [ "$got" != "PASS_ADVISORY" ] && [ "$got" != "PASS" ]; then
  echo "FAIL: the chained re-run blocked on pr-ready's own earlier findings: $got"
  sed -n '/## Blocking Findings/,/## Advisory/p' "$third"
  exit 1
fi
if grep -q "Unresolved review blockers" "$third"; then
  echo "FAIL: the passing run still reports 'Unresolved review blockers'"; exit 1
fi

# 4. A task-done blocker on the branch still blocks pr-ready: the exemption is pr-ready's own
#    findings only.
printf '# task\n' > task.md; git add task.md; git -c commit.gpgsign=false commit -qm task
"$BIN" review task-done task.md --base main >/dev/null 2>&1 || true
"$BIN" review record-lenses --scope pr-ready --base main --lenses security --verdict PASS --mode in-session-emulated >/dev/null 2>&1
fourth="$(gate --evidence "$ev")"
if ! grep -q "Unresolved review blockers" "$fourth"; then
  echo "FAIL: a task-done blocker must still block pr-ready (verdict $(verdict "$fourth"))"; exit 1
fi

echo "test-pr-ready-rerun: ok"
