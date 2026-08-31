#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

go test ./...

version="$(go run ./cmd/metareview --version)"
test "$version" = "$(node -p "require('./package.json').version")"

help="$(go run ./cmd/metareview --help)"
printf '%s\n' "$help" | grep -q 'metareview setup --check'
printf '%s\n' "$help" | grep -q 'metareview context build <path>'
printf '%s\n' "$help" | grep -q 'metareview context diff'
printf '%s\n' "$help" | grep -q 'metareview evidence run --'
printf '%s\n' "$help" | grep -q 'metareview evidence import --github-checks'
printf '%s\n' "$help" | grep -q 'metareview review artifact <path>'
printf '%s\n' "$help" | grep -q -- '--scaffold-only'
printf '%s\n' "$help" | grep -q 'metareview review task-done'
printf '%s\n' "$help" | grep -q 'metareview review epic-ready'
printf '%s\n' "$help" | grep -q 'metareview review pr-ready'

# status --json is the contract a host hook branches on: valid JSON on stdout, a `blocked`
# flag, and exit 1 when something must be cleared so the common decision needs no parsing.
# Exercised here rather than in a Go test because the exit decision lives in main's dispatch.
clean="$(mktemp -d)"
trap 'rm -rf "$clean"' EXIT
go build -o "$clean/mrv" ./cmd/metareview

out="$(cd "$clean" && ./mrv status --json)" || { echo "FAIL: status --json on a clean tree exited nonzero"; exit 1; }
printf '%s' "$out" | grep -q '"blocked": false' || { echo "FAIL: clean tree must not be blocked"; exit 1; }
printf '%s' "$out" | grep -q '"must_clear": \[\]' || { echo "FAIL: clean tree must have an empty must_clear"; exit 1; }

mkdir -p "$clean/docs/metareview/reviews"
printf '# metareview: task-done review\n\nRun ID: `mrv-x`\nTarget: `t-1`\n\n## Verdict\n\nNEEDS_REVISION\n' \
  > "$clean/docs/metareview/reviews/mrv-x-task-done-t-1.md"
if (cd "$clean" && ./mrv status --json >/dev/null); then
  echo "FAIL: status --json must exit nonzero when something must be cleared"; exit 1
fi
# the binary exits 1 by design here, and pipefail would fail the pipeline before grep runs
blocked_out="$( (cd "$clean" && ./mrv status --json 2>/dev/null) || true )"
printf '%s' "$blocked_out" | grep -q 't-1' || { echo "FAIL: the blocker must be named in the output"; exit 1; }
printf '%s' "$blocked_out" | grep -q '"blocked": true' || { echo "FAIL: blocked must be true"; exit 1; }

# --target scopes that answer to the work in hand. Without it `blocked` spans the whole review
# history, so a Stop hook wired to it refuses an agent because of work it never touched: a
# livelock rather than a gate, and the reason the hooks could not ship before now.
printf '# metareview: task-done review\n\nRun ID: `mrv-y`\nTarget: `t-2`\n\n## Verdict\n\nNEEDS_REVISION\n' \
  > "$clean/docs/metareview/reviews/mrv-y-task-done-t-2.md"

scoped="$( (cd "$clean" && ./mrv status --json --target t-1 2>/dev/null) || true )"
printf '%s' "$scoped" | grep -q 't-1' || { echo "FAIL: the scoped report must name its own blocker"; exit 1; }
printf '%s' "$scoped" | grep -q 't-2' && { echo "FAIL: scoping must not leak another target's blocker"; exit 1; }
printf '%s' "$scoped" | grep -q '"target": "t-1"' || { echo "FAIL: the report must say what it was scoped to"; exit 1; }

# A target NO REVIEW COVERS is blocked, as UNREVIEWED. This assertion used to be the opposite,
# and that was the hole: the narrower the scope an agent claimed, the more certainly the gate let
# it through, because an unknown target matched no log and produced an empty must_clear. Naming a
# file nobody had reviewed was the reliable way to be told everything was fine.
unreviewed="$( (cd "$clean" && ./mrv status --json --target t-untouched 2>/dev/null) || true )"
printf '%s' "$unreviewed" | grep -q 'UNREVIEWED' || {
  echo "FAIL: a target no review covers must be reported UNREVIEWED, not cleared"; exit 1; }
if (cd "$clean" && ./mrv status --json --target t-untouched >/dev/null 2>&1); then
  echo "FAIL: a target no review covers must not exit 0"; exit 1
fi

# ...and a target that WAS reviewed and passed still lets work through, or the gate is a livelock.
printf '# metareview: task-done review\n\nRun ID: `mrv-z`\nTarget: `t-3`\n\n## Verdict\n\nPASS\n' \
  > "$clean/docs/metareview/reviews/mrv-z-task-done-t-3.md"
if ! (cd "$clean" && ./mrv status --json --target t-3 >/dev/null 2>&1); then
  echo "FAIL: a reviewed, passing target must exit 0"; exit 1
fi

# A real branch, so --scope branch has something to resolve. gitcontext takes the base from
# merge-base with main, so the work has to be on a branch OFF main, not on main itself.
(
  cd "$clean"
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
)

# --scope branch answers about the work in hand: this branch's own commits, and the files it
# changed that no passing review has read. It is the scope a Stop hook needs — unscoped, the
# answer spans the whole review history and refuses every session for work it never touched.
if (cd "$clean" && ./mrv status --json --scope branch >/dev/null 2>&1); then
  echo "FAIL: a branch whose changed files nobody reviewed must block"; exit 1
fi
branch_out="$( (cd "$clean" && ./mrv status --json --scope branch 2>/dev/null) || true )"
printf '%s' "$branch_out" | grep -q 'UNREVIEWED' || {
  echo "FAIL: changed files with no covering review must be reported UNREVIEWED"; exit 1; }
printf '%s' "$branch_out" | grep -q '"target": "branch ' || {
  echo "FAIL: the report must say it was scoped to a branch"; exit 1; }
# t-1's blocker is not on this branch's commits, so it must not appear in the branch answer.
printf '%s' "$branch_out" | grep -q '"run_id": "mrv-x"' && {
  echo "FAIL: an unrelated blocker leaked into the branch-scoped answer"; exit 1; }
# The spelling with an equals sign works too. The `|| true` this used to carry discarded the
# result, so the form was invoked and never actually asserted.
eq_out="$( (cd "$clean" && ./mrv status --json --scope=branch 2>/dev/null) || true )"
printf '%s' "$eq_out" | grep -q '"target": "branch ' || {
  echo "FAIL: --scope=branch must scope the same way --scope branch does: $eq_out"; exit 1; }
# And an explicit base, which had no end-to-end coverage at all.
base_sha="$( (cd "$clean" && git rev-parse HEAD~1) 2>/dev/null || true )"
if [ -n "$base_sha" ]; then
  base_out="$( (cd "$clean" && ./mrv status --json --scope branch --base "$base_sha" 2>/dev/null) || true )"
  printf '%s' "$base_out" | grep -q "\"target\": \"branch $(printf '%s' "$base_sha" | cut -c1-8)" || {
    echo "FAIL: --base must set the base the report names: $base_out"; exit 1; }
fi
# A malformed scope is refused rather than silently treated as unscoped.
if (cd "$clean" && ./mrv status --json --scope 2>/dev/null); then
  echo "FAIL: --scope with no value must be refused"; exit 1
fi
if (cd "$clean" && ./mrv status --json --scope nonsense 2>/dev/null); then
  echo "FAIL: an unknown scope must be refused"; exit 1
fi

# The gate must also survive being run from a subdirectory, which is where a Stop hook actually
# runs: it inherits the session's cwd, and resolving there used to find no review logs at all.
mkdir -p "$clean/internal/deep"
if (cd "$clean/internal/deep" && "$clean/mrv" status --json >/dev/null 2>&1); then
  echo "FAIL: status from a subdirectory must still see the repository's blockers"; exit 1
fi
# ...while the unscoped answer over the same tree still blocks.
if (cd "$clean" && ./mrv status --json >/dev/null 2>&1); then
  echo "FAIL: the unscoped answer must still block"; exit 1
fi
# A malformed invocation is refused rather than silently treated as unscoped.
if (cd "$clean" && ./mrv status --json --target 2>/dev/null); then
  echo "FAIL: --target with no value must be refused"; exit 1
fi
