#!/usr/bin/env bash
set -euo pipefail

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
