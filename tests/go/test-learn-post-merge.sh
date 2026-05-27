#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
printf ".metareview/\n" > .gitignore
printf "old\n" > README.md
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "new\n" > README.md
git add .
git commit -qm "change"

"$TMP/metareview" learn > "$TMP/learn-help.out"
grep -q "metareview learn --post-merge" "$TMP/learn-help.out"
test ! -e .metareview
test ! -e docs/metareview/learning

mkdir -p .metareview "$TMP/sessions"
cat > .metareview/findings.jsonl <<'JSONL'
{"schemaVersion":1,"id":"mrvf-accept","runId":"mrv-run","reviewer":"architecture-reviewer","severity":"high","classification":"blocking","status":"fixed","title":"Duplicate service path","finding":"Added a duplicate service path.","expected":"Reuse inventoried service paths.","recommendation":"Before adding a new service path, reviewers should check SERVICE_INVENTORY and require reuse or an explicit split rationale.","knowledgeCandidate":true,"fingerprint":"dup","fixedInRunId":"mrv-fixed"}
{"schemaVersion":1,"id":"mrvf-discard","runId":"mrv-run","reviewer":"test-reviewer","severity":"high","classification":"blocking","status":"fixed","title":"Run tests","finding":"Tests were missing.","recommendation":"Run tests before finishing.","knowledgeCandidate":true,"fingerprint":"tests","fixedInRunId":"mrv-fixed"}
{"schemaVersion":1,"id":"mrvf-cal","runId":"mrv-run","reviewer":"security-reviewer","severity":"medium","classification":"false-positive","status":"false-positive","title":"False positive eval finding","finding":"The flagged eval path was test fixture data.","fingerprint":"fp"}
JSONL
printf '{"timestamp":"2026-05-26T10:00:00Z","message":"Reviewer correction: preserve original intent after plan iteration."}\n' > "$TMP/sessions/session.jsonl"

"$TMP/metareview" learn --post-merge 7 --base "$base" --session-root "$TMP/sessions" > "$TMP/learn.out"
accepted_path="$(cat "$TMP/learn.out")"
test -f "$accepted_path"
discard_path="${accepted_path%-accepted.md}-discarded.md"
test -f "$discard_path"
grep -q "Accepted Learning" "$accepted_path"
grep -q "Before adding a new service path" "$accepted_path"
grep -q "Reviewer correction" "$accepted_path"
grep -q "GitHub context unavailable" "$accepted_path"
grep -q "Session history: available" "$accepted_path"
grep -q "Discarded Candidates" "$discard_path"
grep -q "self-evident" "$discard_path"
test -f .metareview/learning-runs.jsonl
test -f .metareview/knowledge/metareview.jsonl
test -f .metareview/calibration.jsonl
grep -q "mrvf-accept" .metareview/knowledge/metareview.jsonl
grep -q "false-positive" .metareview/calibration.jsonl
! git check-ignore -q .metareview/knowledge/metareview.jsonl
! git check-ignore -q .metareview/calibration.jsonl
! git check-ignore -q .metareview/learning-runs.jsonl
git check-ignore -q .metareview/findings.jsonl
git check-ignore -q .metareview/runs.jsonl

set +e
"$TMP/metareview" learn --post-merge > "$TMP/missing.out" 2>"$TMP/missing.err"
missing_code=$?
set -e
test "$missing_code" -eq 2
grep -q "Missing value for --post-merge" "$TMP/missing.err"

failure="$TMP/failure"
mkdir -p "$failure"
cd "$failure"
git init -q
git config user.email test-user
git config user.name "Test User"
printf "old\n" > README.md
git add .
git commit -qm "initial"
failure_base="$(git rev-parse HEAD)"
printf "new\n" > README.md
git add .
git commit -qm "change"
mkdir -p .metareview/calibration.jsonl
cat > .metareview/findings.jsonl <<'JSONL'
{"schemaVersion":1,"id":"mrvf-accept","runId":"mrv-run","reviewer":"architecture-reviewer","severity":"high","classification":"blocking","status":"fixed","title":"Duplicate service path","finding":"Added a duplicate service path.","expected":"Reuse inventoried service paths.","recommendation":"Before adding a new service path, reviewers should check SERVICE_INVENTORY and require reuse or an explicit split rationale.","knowledgeCandidate":true,"fingerprint":"dup","fixedInRunId":"mrv-fixed"}
{"schemaVersion":1,"id":"mrvf-cal","runId":"mrv-run","reviewer":"security-reviewer","severity":"medium","classification":"false-positive","status":"false-positive","title":"False positive eval finding","finding":"The flagged eval path was test fixture data.","fingerprint":"fp"}
JSONL
set +e
"$TMP/metareview" learn --post-merge 8 --base "$failure_base" > "$TMP/failure.out" 2>"$TMP/failure.err"
failure_code=$?
set -e
test "$failure_code" -ne 0
test ! -e docs/metareview/learning
test ! -f .metareview/learning-runs.jsonl
test ! -e .metareview/knowledge
