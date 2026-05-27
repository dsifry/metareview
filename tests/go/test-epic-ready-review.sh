#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

write_issue() {
  node -e 'const issue = JSON.parse(process.argv[1]); console.log(JSON.stringify(issue));' "$1"
}

repo="$TMP/contradiction"
mkdir -p "$repo/.beads" "$repo/docs"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
{
  write_issue '{"id":"epic-1","title":"Expression parser","description":"Parse expressions without executing input","children":["task-use-eval","task-avoid-eval"]}'
  write_issue '{"id":"task-use-eval","title":"Eval parser","description":"Use eval for expression parsing."}'
  write_issue '{"id":"task-avoid-eval","title":"Safe parser","description":"Avoid eval and parse tokens directly."}'
} > .beads/issues.jsonl
printf "initial\n" > docs/change.md
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "updated\n" > docs/change.md
git add .
git commit -qm "change"

set +e
"$TMP/metareview" review epic-ready epic-1 --base "$base" > "$TMP/contradiction.out" 2>"$TMP/contradiction.err"
code=$?
set -e
test "$code" -eq 1
contradiction_review="$(cat "$TMP/contradiction.out")"
test -f "$repo/$contradiction_review"
grep -q "NEEDS_REVISION" "$repo/$contradiction_review"
grep -q "Cross-task contradiction" "$repo/$contradiction_review"
grep -q "mrvf-" "$repo/docs/metareview/FINDINGS.md"

repo="$TMP/unresolved"
mkdir -p "$repo/.beads" "$repo/docs/metareview/reviews"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
{
  write_issue '{"id":"epic-2","title":"Child blockers","description":"Close only after children pass.","children":["task-1"]}'
  write_issue '{"id":"task-1","title":"Child task","description":"Implement child safely."}'
} > .beads/issues.jsonl
cat > docs/metareview/reviews/child.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-child`

Target: `task-1`

## Verdict

NEEDS_REVISION

## Findings

### mrvf-child-001: Existing blocker
REVIEW
printf "initial\n" > docs/change.md
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "updated\n" > docs/change.md
git add .
git commit -qm "change"

set +e
"$TMP/metareview" review epic-ready epic-2 --base "$base" > "$TMP/unresolved.out" 2>"$TMP/unresolved.err"
code=$?
set -e
test "$code" -eq 1
unresolved_review="$(cat "$TMP/unresolved.out")"
grep -q "Unresolved child blockers" "$repo/$unresolved_review"

repo="$TMP/incomplete-child-artifact"
mkdir -p "$repo/.beads" "$repo/docs/metareview/reviews"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
{
  write_issue '{"id":"epic-2b","title":"Child artifact incomplete","description":"Close only after child review passes.","children":["task-1"]}'
  write_issue '{"id":"task-1","title":"Child task","description":"Implement child safely."}'
} > .beads/issues.jsonl
cat > docs/metareview/reviews/child-artifact.md <<'REVIEW'
# metareview: artifact review

Run ID: `mrv-child-artifact`

Target: `task-1`

## Verdict

NOT_REVIEWED

## Reviewer Results

| Reviewer | Verdict | Blocking | Warnings | Notes |
| --- | --- | ---: | ---: | --- |

## Findings

No reviewer findings recorded yet.
REVIEW
printf "initial\n" > docs/change.md
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "updated\n" > docs/change.md
git add .
git commit -qm "change"

set +e
"$TMP/metareview" review epic-ready epic-2b --base "$base" > "$TMP/incomplete-child-artifact.out" 2>"$TMP/incomplete-child-artifact.err"
code=$?
set -e
test "$code" -eq 1
incomplete_child_artifact_review="$(cat "$TMP/incomplete-child-artifact.out")"
grep -q "Unresolved child blockers" "$repo/$incomplete_child_artifact_review"

repo="$TMP/clean"
mkdir -p "$repo/.beads" "$repo/docs/metareview/reviews" "$repo/docs"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
{
  write_issue '{"id":"epic-3","title":"Clean epic","description":"Ship documented parser work.","children":["task-1","task-2"]}'
  write_issue '{"id":"task-1","title":"Parser","description":"Implement parser."}'
  write_issue '{"id":"task-2","title":"Tests","description":"Add tests."}'
} > .beads/issues.jsonl
cat > docs/metareview/reviews/task-1-old.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-20250101-000000000000000-task-done-task-1-old`

Target: `task-1`

## Verdict

NEEDS_REVISION

## Findings

### mrvf-task-1-old-001: Fixed blocker
REVIEW
cat > docs/metareview/reviews/task-1.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-20250101-000100000000000-task-done-task-1-new`

Target: `task-1`

## Verdict

PASS
REVIEW
cat > docs/metareview/reviews/task-2.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-task-2`

Target: `task-2`

## Verdict

PASS
REVIEW
printf "initial\n" > docs/change.md
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "updated\n" > docs/change.md
git add .
git commit -qm "change"

"$TMP/metareview" review epic-ready epic-3 --base "$base" > "$TMP/clean.out"
clean_review="$(cat "$TMP/clean.out")"
grep -q "PASS" "$repo/$clean_review"
grep -q "No blocking findings." "$repo/$clean_review"

set +e
"$TMP/metareview" review epic-ready epic-3 --base > "$TMP/missing.out" 2>"$TMP/missing.err"
missing_code=$?
set -e
test "$missing_code" -eq 2
grep -q "Missing value for --base" "$TMP/missing.err"

repo="$TMP/failure"
mkdir -p "$repo/.beads" "$repo/docs"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
{
  write_issue '{"id":"epic-4","title":"Failure fixture","description":"Trigger rollback.","children":["task-1"]}'
  write_issue '{"id":"task-1","title":"Child","description":"Use eval for parsing."}'
} > .beads/issues.jsonl
printf "initial\n" > docs/change.md
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "updated\n" > docs/change.md
git add .
git commit -qm "change"
printf "not a directory\n" > .metareview
set +e
"$TMP/metareview" review epic-ready epic-4 --base "$base" > "$TMP/failure.out" 2>"$TMP/failure.err"
failure_code=$?
set -e
test "$failure_code" -ne 0
test ! -d docs/metareview/context
test ! -f docs/metareview/FINDINGS.md
