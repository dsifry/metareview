#!/usr/bin/env bash
# The Stop hook: the shim that makes the Completion Rule a gate rather than a sentence in
# CLAUDE.md. It had no test at all, which is how it shipped defaulting to an UNSCOPED status
# query — a livelock nobody could clear — while `--scope branch` already existed beside it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
HOOK="$ROOT/hooks/pre-finish.sh"

(cd "$ROOT" && go build -o "$TMP/mrv" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo/docs/metareview/reviews"
cd "$repo"
git init -q -b main
git config user.email t@e
git config user.name t
printf 'package p\n' > base.go
git add base.go
git -c commit.gpgsign=false commit -qm base
git checkout -q -b work
printf 'package p // changed\n' > changed.go
git add changed.go
git -c commit.gpgsign=false commit -qm work
head="$(git rev-parse HEAD)"

# A hook must always emit ONE line of valid JSON when it blocks, or the host cannot act on it.
assert_json_block() {  # assert_json_block <output> <substring>
  printf '%s' "$1" | python3 -c '
import json, sys
d = json.load(sys.stdin)
assert d["decision"] == "block", d
assert d["reason"], d
' || { echo "FAIL: hook did not emit a valid block decision: $1"; exit 1; }
  printf '%s' "$1" | grep -q "$2" || { echo "FAIL: reason missing %s: $1" "$2"; exit 1; }
}

# 1. Absent tooling blocks. A check that did not run must never read as a check that found
#    nothing wrong.
out="$(METAREVIEW_BIN=definitely-not-installed bash "$HOOK")"
assert_json_block "$out" "not installed"

# 2. The branch changed a file no review has read, so the hook blocks — and says so in the words
#    that tell an operator what to do, distinctly from "you have findings to fix".
out="$(METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "never been reviewed"
printf '%s' "$out" | grep -q "changed.go" || { echo "FAIL: the block must name the unreviewed file: $out"; exit 1; }

# 3. Once a passing review records that it read the file, the hook lets the session finish. A
#    gate that cannot be satisfied is one an operator disables, which is worse than none.
printf '# metareview: pr-ready review\n\nRun ID: `mrv-1`\n\nTarget: `current branch`\n\nHead: `%s`\n\nCovered paths: `base.go, changed.go`\n\n## Verdict\n\nPASS\n' \
  "$head" > docs/metareview/reviews/mrv-1-pr-ready.md
out="$(METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
if [ -n "$out" ]; then echo "FAIL: a reviewed branch must pass silently, got: $out"; exit 1; fi

# 4. A blocking review of this branch's own commits blocks, and is reported as a verdict rather
#    than as "never reviewed" — the two call for different things from whoever reads it.
printf '# metareview: task-done review\n\nRun ID: `mrv-2`\n\nTarget: `t-2`\n\nHead: `%s`\n\nCovered paths: `changed.go`\n\n## Verdict\n\nNEEDS_REVISION\n' \
  "$head" > docs/metareview/reviews/mrv-2-task-done.md
out="$(METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "NEEDS_REVISION"

# 5. METAREVIEW_TARGET still narrows to one target when the caller knows it.
out="$(METAREVIEW_BIN="$TMP/mrv" METAREVIEW_TARGET=t-2 bash "$HOOK")"
assert_json_block "$out" "t-2"

# 6. The hook runs from wherever the session happens to be standing. Resolving against the
#    process cwd found no review logs at all and exited 0 — the gate bypassed by the entirely
#    ordinary act of working in a subdirectory, and bypassed silently.
mkdir -p "$repo/internal/deep"
out="$(cd "$repo/internal/deep" && METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "NEEDS_REVISION"

# 7. A gate that errors is broken, not clean, and says which of the two it is.
cat > "$TMP/broken" <<'EOF'
#!/bin/sh
echo "boom" >&2
exit 7
EOF
chmod +x "$TMP/broken"
out="$(METAREVIEW_BIN="$TMP/broken" bash "$HOOK")"
assert_json_block "$out" "could not answer"

echo "test-stop-hook: ok"
