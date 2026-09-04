#!/usr/bin/env bash
set -euo pipefail

# This script exercises the DETERMINISTIC pr-ready/task-done layer (evidence, blockers, verdicts).
# The adversarial-review requirement (build B) is tested separately in test-require-adjudicated-review.sh,
# so opt out of it here to keep these assertions focused.
export METAREVIEW_ALLOW_MECHANICAL_PASS=1

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

init_repo() {
  local repo="$1"
  mkdir -p "$repo/lib" "$repo/tests"
  cd "$repo"
  git init -q
  git config user.email test-user
  git config user.name "Test User"
  printf "'use strict';\nmodule.exports = input => JSON.parse(input);\n" > lib/parser.js
  printf "initial\n" > tests/parser.test.txt
  git add .
  git commit -qm "initial"
}

write_mock_gh() {
  local bin="$1"
  mkdir -p "$bin"
  cat > "$bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  cat <<'JSON'
{"number":99,"url":"https://github.com/acme/repo/pull/99","title":"Safe parser PR","body":"Ready for review","comments":[{"author":{"login":"alice"},"url":"https://github.com/acme/repo/pull/99#issuecomment-1","body":"Looks ready"}],"reviews":[]}
JSON
  exit 0
fi
exit 1
GH
  chmod +x "$bin/gh"
}

repo="$TMP/unresolved"
init_repo "$repo"
base="$(git rev-parse HEAD)"
mkdir -p docs/metareview/reviews
cat > docs/metareview/reviews/task-blocked.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-task-blocked`

Target: `task-1`

## Verdict

NEEDS_REVISION

## Findings

### mrvf-task-001: Existing blocker
REVIEW
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // unresolved branch\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/evidence.md" > "$TMP/unresolved.out" 2>"$TMP/unresolved.err"
code=$?
set -e
test "$code" -eq 1
unresolved_review="$(cat "$TMP/unresolved.out")"
grep -q "Unresolved review blockers" "$repo/$unresolved_review"

repo="$TMP/incomplete-artifact"
init_repo "$repo"
base="$(git rev-parse HEAD)"
mkdir -p docs/metareview/reviews
cat > docs/metareview/reviews/spec-not-reviewed.md <<'REVIEW'
# metareview: artifact review

Run ID: `mrv-spec-not-reviewed`

Target: `lib/parser.js`

## Verdict

NOT_REVIEWED

## Reviewer Results

| Reviewer | Verdict | Blocking | Warnings | Notes |
| --- | --- | ---: | ---: | --- |

## Findings

No reviewer findings recorded yet.
REVIEW
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // incomplete artifact\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/incomplete-artifact-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/incomplete-artifact-evidence.md" > "$TMP/incomplete-artifact.out" 2>"$TMP/incomplete-artifact.err"
code=$?
set -e
test "$code" -eq 1
incomplete_artifact_review="$(cat "$TMP/incomplete-artifact.out")"
grep -q "Unresolved review blockers" "$repo/$incomplete_artifact_review"
grep -q "lib/parser.js" "$repo/$incomplete_artifact_review"

repo="$TMP/unrelated-artifact"
init_repo "$repo"
base="$(git rev-parse HEAD)"
mkdir -p docs/metareview/reviews
cat > docs/metareview/reviews/spec-not-reviewed.md <<'REVIEW'
# metareview: artifact review

Run ID: `mrv-spec-not-reviewed`

Target: `docs/spec.md`

## Verdict

NOT_REVIEWED

## Reviewer Results

| Reviewer | Verdict | Blocking | Warnings | Notes |
| --- | --- | ---: | ---: | --- |

## Findings

No reviewer findings recorded yet.
REVIEW
mkdir -p .metareview
cat > .metareview/findings.jsonl <<'JSONL'
{"schemaVersion":1,"id":"mrvf-spec-not-reviewed-001","runId":"mrv-spec-not-reviewed","status":"open","classification":"blocking","severity":"high","title":"Unrelated artifact blocker","target":{"type":"path","path":"docs/spec.md"}}
JSONL
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // unrelated artifact\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/unrelated-artifact-evidence.md"
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/unrelated-artifact-evidence.md" > "$TMP/unrelated-artifact.out"
unrelated_artifact_review="$(cat "$TMP/unrelated-artifact.out")"
grep -q "PASS" "$repo/$unrelated_artifact_review"
grep -q "Unresolved review blockers" "$repo/$unrelated_artifact_review" && exit 1
grep -q "Repository Health Advisory" "$repo/$unrelated_artifact_review"
grep -q "Unrelated artifact blocker (docs/spec.md)" "$repo/$unrelated_artifact_review"

repo="$TMP/unrelated-artifact-working-tree"
init_repo "$repo"
base="$(git rev-parse HEAD)"
mkdir -p docs/metareview/reviews .metareview
cat > docs/metareview/reviews/spec-not-reviewed.md <<'REVIEW'
# metareview: artifact review

Run ID: `mrv-spec-not-reviewed`

Target: `docs/spec.md`

## Verdict

NOT_REVIEWED

## Reviewer Results

| Reviewer | Verdict | Blocking | Warnings | Notes |
| --- | --- | ---: | ---: | --- |

## Findings

No reviewer findings recorded yet.
REVIEW
cat > .metareview/findings.jsonl <<'JSONL'
{"schemaVersion":1,"id":"mrvf-spec-not-reviewed-001","runId":"mrv-spec-not-reviewed","status":"open","classification":"blocking","severity":"high","title":"Unrelated artifact blocker","target":{"type":"path","path":"docs/spec.md"}}
JSONL
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // working tree artifact relevance\n" > lib/parser.js
printf "bash tests/run-all.sh exited 0\n" > "$TMP/unrelated-artifact-working-tree-evidence.md"
"$TMP/metareview" review pr-ready --base "$base" --include-working-tree --evidence "$TMP/unrelated-artifact-working-tree-evidence.md" > "$TMP/unrelated-artifact-working-tree.out"
unrelated_artifact_working_tree_review="$(cat "$TMP/unrelated-artifact-working-tree.out")"
grep -q "PASS" "$repo/$unrelated_artifact_working_tree_review"
grep -q "Unresolved review blockers" "$repo/$unrelated_artifact_working_tree_review" && exit 1
grep -q "Repository Health Advisory" "$repo/$unrelated_artifact_working_tree_review"
grep -q "Unrelated artifact blocker (docs/spec.md)" "$repo/$unrelated_artifact_working_tree_review"

repo="$TMP/unrelated-path-finding-no-log"
init_repo "$repo"
base="$(git rev-parse HEAD)"
mkdir -p .metareview
cat > .metareview/findings.jsonl <<'JSONL'
{"schemaVersion":1,"id":"mrvf-path-001","runId":"mrv-path","status":"open","classification":"blocking","severity":"high","title":"Unrelated path blocker","target":{"type":"path","path":"docs/spec.md"}}
JSONL
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // unrelated path finding without log\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/unrelated-path-finding-no-log-evidence.md"
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/unrelated-path-finding-no-log-evidence.md" > "$TMP/unrelated-path-finding-no-log.out"
unrelated_path_finding_no_log_review="$(cat "$TMP/unrelated-path-finding-no-log.out")"
grep -q "PASS" "$repo/$unrelated_path_finding_no_log_review"
grep -q "Unresolved review blockers" "$repo/$unrelated_path_finding_no_log_review" && exit 1
grep -q "Repository Health Advisory" "$repo/$unrelated_path_finding_no_log_review"
grep -q "Unrelated path blocker (docs/spec.md)" "$repo/$unrelated_path_finding_no_log_review"

repo="$TMP/wrong-scope-previous"
init_repo "$repo"
base="$(git rev-parse HEAD)"
mkdir -p docs/metareview/reviews
cat > docs/metareview/reviews/task-blocked.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-task-blocked`

Target: `task-1`

## Verdict

NEEDS_REVISION

## Findings

### mrvf-task-001: Existing task blocker
REVIEW
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // wrong previous scope\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/wrong-scope-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --previous-run mrv-task-blocked --evidence "$TMP/wrong-scope-evidence.md" > "$TMP/wrong-scope.out" 2>"$TMP/wrong-scope.err"
wrong_scope_code=$?
set -e
test "$wrong_scope_code" -ne 0
test ! -s "$TMP/wrong-scope.out"
grep -q "previous run mrv-task-blocked not found" "$TMP/wrong-scope.err"

repo="$TMP/missing-validation"
init_repo "$repo"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // missing validation\n" > lib/parser.js
git add .
git commit -qm "branch change"
set +e
"$TMP/metareview" review pr-ready --base "$base" > "$TMP/missing-validation.out" 2>"$TMP/missing-validation.err"
code=$?
set -e
test "$code" -eq 1
missing_review="$(cat "$TMP/missing-validation.out")"
grep -q "Missing validation evidence" "$repo/$missing_review"
previous_missing_run="$(basename "$missing_review" .md)"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/fixed-validation.md"
"$TMP/metareview" review pr-ready --base "$base" --previous-run "$previous_missing_run" --evidence "$TMP/fixed-validation.md" > "$TMP/fixed-validation.out"
fixed_review="$(cat "$TMP/fixed-validation.out")"
grep -q "PASS" "$repo/$fixed_review"
grep -q 'Execution mode: `deterministic-local`' "$repo/$fixed_review"
grep -q "Unresolved review blockers" "$repo/$fixed_review" && exit 1
grep -q "Missing validation evidence" "$repo/$fixed_review" && exit 1

repo="$TMP/target-aware"
init_repo "$repo"
base="$(git rev-parse HEAD)"
git checkout -qb feature-target-aware
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // target aware\n" > lib/parser.js
git add .
git commit -qm "target-aware branch change"
mkdir -p docs/metareview/reviews .metareview
cat > docs/metareview/reviews/old-task.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-old-task`

Target: `GUIDE-old`

Covered paths: `["docs/old.md"]`

## Verdict

NEEDS_REVISION
REVIEW
cat > docs/metareview/reviews/current-task.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-current-task`

Target: `GUIDE-current`

Covered paths: `["lib/parser.js"]`

## Verdict

NEEDS_REVISION
REVIEW
cat > docs/metareview/reviews/prior-pass.md <<'REVIEW'
# metareview: pr-ready review

Run ID: `mrv-prior-pass`

Target: `current branch`

Covered paths: `["lib/parser.js"]`

## Verdict

PASS
REVIEW
cat > .metareview/findings.jsonl <<'JSONL'
{"schemaVersion":1,"id":"mrvf-old-task","runId":"mrv-old-task","status":"open","classification":"blocking","severity":"high","title":"Old unrelated task blocker","target":{"type":"beads-task","id":"GUIDE-old"}}
{"schemaVersion":1,"id":"mrvf-current-task","runId":"mrv-current-task","status":"open","classification":"blocking","severity":"high","title":"Current linked task blocker","target":{"type":"beads-task","id":"GUIDE-current"}}
{"schemaVersion":1,"id":"mrvf-other-branch","runId":"mrv-other-branch","status":"open","classification":"blocking","severity":"high","title":"Other branch blocker","target":{"type":"branch","id":"other-branch"}}
JSONL
printf "bash tests/run-all.sh exited 0\n" > "$TMP/target-aware-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/target-aware-evidence.md" > "$TMP/target-aware.out" 2> "$TMP/target-aware.err"
target_aware_code=$?
set -e
test "$target_aware_code" -eq 1
target_aware_review="$(cat "$TMP/target-aware.out")"
grep -q "Current linked task blocker" "$repo/$target_aware_review"
grep -q "Old unrelated task blocker (GUIDE-old)" "$repo/$target_aware_review"
grep -q "Other branch blocker (other-branch)" "$repo/$target_aware_review"
grep -q "Old unrelated task blocker.*does not block this target" "$repo/$target_aware_review"

repo="$TMP/unchanged-reuse"
init_repo "$repo"
git remote add origin https://github.com/acme/repo.git
base="$(git rev-parse HEAD)"
git checkout -qb feature-reuse
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // reusable\n" > lib/parser.js
git add .
git commit -qm "reusable branch change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/reuse-evidence.md"
write_mock_gh "$TMP/reuse-bin"
PATH="$TMP/reuse-bin:$PATH" "$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/reuse-evidence.md" --github-pr 99 > "$TMP/reuse-first.out"
reuse_first_review="$(cat "$TMP/reuse-first.out")"
reuse_first_run="$(basename "$reuse_first_review" .md)"
PATH="$TMP/reuse-bin:$PATH" "$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/reuse-evidence.md" --github-pr 99 > "$TMP/reuse-second.out"
reuse_second_review="$(cat "$TMP/reuse-second.out")"
grep -q "Execution mode: \`deterministic-local-reused\`" "$repo/$reuse_second_review"
grep -q "Reused verdict from: \`$reuse_first_run\`" "$repo/$reuse_second_review"
grep -q "reviewer-input digest is unchanged" "$repo/$reuse_second_review"
tail -n 1 .metareview/runs.jsonl | grep -q '"executionMode":"deterministic-local-reused"'
tail -n 1 .metareview/runs.jsonl | grep -q "\"reusedFromRunId\":\"$reuse_first_run\""

repo="$TMP/cross-branch-current-log"
init_repo "$repo"
base="$(git rev-parse HEAD)"
git checkout -qb branch-a
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch a blocker\n" > lib/parser.js
git add .
git commit -qm "branch a change"
set +e
"$TMP/metareview" review pr-ready --base "$base" > "$TMP/current-log-branch-a.out" 2>"$TMP/current-log-branch-a.err"
current_log_branch_a_code=$?
set -e
test "$current_log_branch_a_code" -eq 1
git checkout -qb branch-b "$base"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch b valid\n" > lib/parser.js
git add .
git commit -qm "branch b change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/current-log-branch-b-evidence.md"
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/current-log-branch-b-evidence.md" > "$TMP/current-log-branch-b.out"
current_log_branch_b_review="$(cat "$TMP/current-log-branch-b.out")"
grep -q "PASS" "$repo/$current_log_branch_b_review"
grep -q "Unresolved review blockers" "$repo/$current_log_branch_b_review" && exit 1

repo="$TMP/cross-branch-previous"
init_repo "$repo"
base="$(git rev-parse HEAD)"
git checkout -qb branch-a
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch a missing validation\n" > lib/parser.js
git add .
git commit -qm "branch a change"
set +e
"$TMP/metareview" review pr-ready --base "$base" > "$TMP/branch-a.out" 2>"$TMP/branch-a.err"
branch_a_code=$?
set -e
test "$branch_a_code" -eq 1
branch_a_review="$(cat "$TMP/branch-a.out")"
branch_a_previous_run="$(basename "$branch_a_review" .md)"
git checkout -qb branch-b "$base"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch b fixed validation\n" > lib/parser.js
git add .
git commit -qm "branch b change"
rm .metareview/runs.jsonl
printf "bash tests/run-all.sh exited 0\n" > "$TMP/branch-b-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --previous-run "$branch_a_previous_run" --evidence "$TMP/branch-b-evidence.md" > "$TMP/branch-b.out" 2>"$TMP/branch-b.err"
branch_b_code=$?
set -e
test "$branch_b_code" -ne 0
test ! -s "$TMP/branch-b.out"
grep -q "previous run $branch_a_previous_run not found" "$TMP/branch-b.err"

repo="$TMP/detached-legacy-previous"
init_repo "$repo"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // branch missing validation before detach\n" > lib/parser.js
git add .
git commit -qm "branch missing validation"
set +e
"$TMP/metareview" review pr-ready --base "$base" > "$TMP/detached-branch.out" 2>"$TMP/detached-branch.err"
detached_branch_code=$?
set -e
test "$detached_branch_code" -eq 1
detached_previous_run="$(basename "$(cat "$TMP/detached-branch.out")" .md)"
head_sha="$(git rev-parse HEAD)"
git checkout -q --detach "$head_sha"
rm .metareview/runs.jsonl
printf "bash tests/run-all.sh exited 0\n" > "$TMP/detached-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --previous-run "$detached_previous_run" --evidence "$TMP/detached-evidence.md" > "$TMP/detached.out" 2>"$TMP/detached.err"
detached_code=$?
set -e
test "$detached_code" -ne 0
test ! -s "$TMP/detached.out"
grep -q "previous run $detached_previous_run not found" "$TMP/detached.err"

repo="$TMP/escalated-legacy-previous"
init_repo "$repo"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // escalated previous\n" > lib/parser.js
git add .
git commit -qm "escalated branch change"
set +e
"$TMP/metareview" review pr-ready --base "$base" --max-attempts 1 > "$TMP/escalated.out" 2>"$TMP/escalated.err"
escalated_code=$?
set -e
test "$escalated_code" -eq 1
escalated_review="$(cat "$TMP/escalated.out")"
grep -q "ESCALATED" "$repo/$escalated_review"
escalated_previous_run="$(basename "$escalated_review" .md)"
rm .metareview/runs.jsonl
printf "bash tests/run-all.sh exited 0\n" > "$TMP/escalated-fixed-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --previous-run "$escalated_previous_run" --evidence "$TMP/escalated-fixed-evidence.md" > "$TMP/escalated-fixed.out" 2>"$TMP/escalated-fixed.err"
escalated_fixed_code=$?
set -e
test "$escalated_fixed_code" -ne 0
test ! -s "$TMP/escalated-fixed.out"
grep -q "previous run $escalated_previous_run already escalated" "$TMP/escalated-fixed.err"
set +e
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/escalated-fixed-evidence.md" > "$TMP/escalated-fresh.out" 2>"$TMP/escalated-fresh.err"
escalated_fresh_code=$?
set -e
test "$escalated_fresh_code" -ne 0
test ! -s "$TMP/escalated-fresh.out"
grep -q "same target already escalated in run $escalated_previous_run" "$TMP/escalated-fresh.err"

repo="$TMP/clean"
init_repo "$repo"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // clean branch\n" > lib/parser.js
printf "parser tests pass\n" > tests/parser.test.txt
git add .
git commit -qm "safe branch"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/clean-evidence.md"
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/clean-evidence.md" --github-pr 99 > "$TMP/clean.out"
clean_review="$(cat "$TMP/clean.out")"
grep -q "PASS" "$repo/$clean_review"
grep -q "metareview PR Evidence" "$repo/$clean_review"
grep -q "GitHub context unavailable" "$repo/$clean_review"

repo="$TMP/dirty-working-tree-default"
init_repo "$repo"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // committed branch\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "'use strict';\nmodule.exports = () => 'local only';\n" > lib/local-only.js
printf "bash tests/run-all.sh exited 0\n" > "$TMP/dirty-default-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/dirty-default-evidence.md" > "$TMP/dirty-default.out" 2>"$TMP/dirty-default.err"
dirty_default_code=$?
set -e
test "$dirty_default_code" -eq 1
dirty_default_review="$(cat "$TMP/dirty-default.out")"
grep -q "Working tree changes excluded from PR-ready review" "$repo/$dirty_default_review"
grep -q "lib/local-only.js" "$repo/$dirty_default_review"

repo="$TMP/dirty-working-tree-included"
init_repo "$repo"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // committed branch\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "'use strict';\nmodule.exports = () => 'local only';\n" > lib/local-only.js
printf "bash tests/run-all.sh exited 0\n" > "$TMP/dirty-included-evidence.md"
"$TMP/metareview" review pr-ready --base "$base" --include-working-tree --evidence "$TMP/dirty-included-evidence.md" > "$TMP/dirty-included.out"
dirty_included_review="$(cat "$TMP/dirty-included.out")"
grep -q "PASS" "$repo/$dirty_included_review"
grep -q "Working tree changes excluded from PR-ready review" "$repo/$dirty_included_review" && exit 1

repo="$TMP/github-available"
init_repo "$repo"
git remote add origin https://github.com/acme/repo.git
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // github branch\n" > lib/parser.js
printf "parser tests pass with github\n" > tests/parser.test.txt
git add .
git commit -qm "github branch"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/github-evidence.md"
write_mock_gh "$TMP/bin"
PATH="$TMP/bin:$PATH" "$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/github-evidence.md" --github-pr 99 > "$TMP/github.out"
github_review="$(cat "$TMP/github.out")"
grep -q "Safe parser PR" "$repo/$github_review"
grep -q "https://github.com/acme/repo/pull/99#issuecomment-1" "$repo/$github_review"

set +e
"$TMP/metareview" review pr-ready --base > "$TMP/missing.out" 2>"$TMP/missing.err"
missing_code=$?
set -e
test "$missing_code" -eq 2
grep -q "Missing value for --base" "$TMP/missing.err"

set +e
"$TMP/metareview" review pr-ready --github-pr > "$TMP/missing-gh.out" 2>"$TMP/missing-gh.err"
missing_gh_code=$?
set -e
test "$missing_gh_code" -eq 2
grep -q "Missing value for --github-pr" "$TMP/missing-gh.err"

repo="$TMP/failure"
init_repo "$repo"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => eval(input);\n" > lib/parser.js
git add .
git commit -qm "unsafe branch"
printf "not a directory\n" > .metareview
set +e
"$TMP/metareview" review pr-ready --base "$base" > "$TMP/failure.out" 2>"$TMP/failure.err"
failure_code=$?
set -e
test "$failure_code" -ne 0
test ! -d docs/metareview/context
test ! -d docs/metareview/reviews
test ! -f docs/metareview/FINDINGS.md
