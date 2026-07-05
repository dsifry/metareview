#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo/lib" "$repo/.beads"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
printf '{"id":"task-1","title":"Safe parser","description":"Parse expressions without executing input","acceptance":["no eval","tests pass"]}\n' > .beads/issues.jsonl
printf "'use strict';\nmodule.exports = input => JSON.parse(input);\n" > lib/parser.js
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => eval(input);\n" > lib/parser.js
git add .
git commit -qm "unsafe parser"

set +e
"$TMP/metareview" review task-done task-1 --base "$base" > "$TMP/blocking.out" 2>"$TMP/blocking.err"
code=$?
set -e
test "$code" -eq 1
review_path="$(cat "$TMP/blocking.out")"
test -f "$repo/$review_path"
grep -q "NEEDS_REVISION" "$repo/$review_path"
grep -q "Unsafe eval introduced" "$repo/$review_path"
grep -q "mrvf-" "$repo/docs/metareview/FINDINGS.md"
first_run="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\\n').map(JSON.parse); console.log(lines[0].id)")"

mkdir -p tests/lib
printf "'use strict';\nmodule.exports = input => JSON.parse(input);\n" > lib/parser.js
printf "parser validation passed\n" > tests/lib/parser.test.txt
git add .
git commit -qm "safe parser"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/evidence.md"

"$TMP/metareview" review task-done task-1 --base "$base" --previous-run "$first_run" --evidence "$TMP/evidence.md" > "$TMP/pass.out"
second_review="$(cat "$TMP/pass.out")"
grep -q "PASS" "$repo/$second_review"
grep -q "No unresolved findings recorded yet." "$repo/docs/metareview/FINDINGS.md"

mkdir -p "$TMP/advisory/lib"
cd "$TMP/advisory"
git init -q
git config user.email test-user
git config user.name "Test User"
printf "'use strict';\nmodule.exports = () => 1;\n" > lib/a.js
git add .
git commit -qm "initial"
printf "'use strict';\nmodule.exports = () => 2;\n" > lib/a.js
git add .
git commit -qm "change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/advisory-evidence.md"
"$TMP/metareview" review task-done unknown-task --base HEAD~1 --evidence "$TMP/advisory-evidence.md" > "$TMP/advisory.out"
grep -q "PASS_ADVISORY" "$(cat "$TMP/advisory.out")"

set +e
"$TMP/metareview" review task-done unknown-task --base > "$TMP/missing.out" 2>"$TMP/missing.err"
missing_code=$?
set -e
test "$missing_code" -eq 2
grep -q "Missing value for --base" "$TMP/missing.err"

set +e
"$TMP/metareview" review task-done unknown-task --base --evidence "$TMP/advisory-evidence.md" > "$TMP/missing2.out" 2>"$TMP/missing2.err"
missing2_code=$?
set -e
test "$missing2_code" -eq 2
grep -q "Missing value for --base" "$TMP/missing2.err"

mkdir -p "$TMP/generated-target/docs/metareview/reviews"
cd "$TMP/generated-target"
git init -q
git config user.email test-user
git config user.name "Test User"
printf "# Generated Review\n\nInitial\n" > docs/metareview/reviews/target.md
mkdir -p docs/metareview/context
printf "Initial noise\n" > docs/metareview/context/noise.md
git add .
git commit -qm "initial"
generated_base="$(git rev-parse HEAD)"
printf "# Generated Review\n\nUpdated review artifact under explicit target.\n" > docs/metareview/reviews/target.md
printf "noise artifact\n%.0s" {1..5000} > docs/metareview/context/noise.md
git add .
git commit -qm "generated target change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/generated-target-evidence.md"
"$TMP/metareview" review task-done docs/metareview/reviews/target.md --base "$generated_base" --evidence "$TMP/generated-target-evidence.md" > "$TMP/generated-target.out"
generated_target_review="$(cat "$TMP/generated-target.out")"
grep -q "docs/metareview/reviews/target.md" "$generated_target_review"
grep -q "docs/metareview/reviews/target.md" "$(dirname "$generated_target_review")/../context/$(basename "$generated_target_review" .md)-context.md"
grep -q "diff --git a/docs/metareview/reviews/target.md b/docs/metareview/reviews/target.md" "$(dirname "$generated_target_review")/../context/$(basename "$generated_target_review" .md)-context.md"
! grep -q "noise artifact" "$(dirname "$generated_target_review")/../context/$(basename "$generated_target_review" .md)-context.md"

mkdir -p "$TMP/failure/lib" "$TMP/failure/.beads"
cd "$TMP/failure"
git init -q
git config user.email test-user
git config user.name "Test User"
printf '{"id":"task-1","title":"Failure fixture","description":"Trigger rollback","acceptance":["no partial state"]}\n' > .beads/issues.jsonl
printf "'use strict';\nmodule.exports = input => JSON.parse(input);\n" > lib/parser.js
git add .
git commit -qm "initial"
failure_base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => eval(input);\n" > lib/parser.js
git add .
git commit -qm "unsafe"
printf "not a directory\n" > .metareview
set +e
"$TMP/metareview" review task-done task-1 --base "$failure_base" > "$TMP/failure.out" 2>"$TMP/failure.err"
failure_code=$?
set -e
test "$failure_code" -ne 0
test ! -d docs/metareview/context
test ! -f docs/metareview/FINDINGS.md
