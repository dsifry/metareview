#!/usr/bin/env bash
# Mutation reports reaching the review gates end to end.
#
# The case that matters is the second one. On 2026-08-29 gremlins reported
# "Killed 10, Lived 0, Test efficacy: 100.00%" and exited 0 for a run in which 97 mutants had
# timed out: its summary has no field for that class, and its efficacy is killed/(killed+lived),
# so the worse the configuration the better the score. metareview must refuse that run.
set -euo pipefail

# This exercises mutation-report handling in the deterministic gate. The adversarial-review requirement
# (build B) is tested separately in test-require-adjudicated-review.sh, so opt out of it here.
export METAREVIEW_ALLOW_MECHANICAL_PASS=1

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

# A gremlins run with real survivors, and the same run's dishonest sibling.
cat > "$TMP/survivors.json" <<'JSON'
{"go_module":"m","files":[{"file_name":"lib/parser.js","mutations":[
  {"type":"CONDITIONALS_NEGATION","status":"KILLED","line":1,"column":1},
  {"type":"INVERT_NEGATIVES","status":"LIVED","line":2,"column":30},
  {"type":"ARITHMETIC_BASE","status":"NOT COVERED","line":3,"column":1}]}]}
JSON
cat > "$TMP/timeouts.json" <<'JSON'
{"go_module":"m","files":[{"file_name":"lib/parser.js","mutations":[
  {"type":"CONDITIONALS_NEGATION","status":"KILLED","line":1,"column":1},
  {"type":"CONDITIONALS_BOUNDARY","status":"TIMED OUT","line":2,"column":1},
  {"type":"INVERT_NEGATIVES","status":"TIMED OUT","line":3,"column":1}]}]}
JSON
# The cross-language schema, which is the node's declared input contract.
cat > "$TMP/stryker.json" <<'JSON'
{"schemaVersion":"1.0","files":{"lib/parser.js":{"mutants":[
  {"mutatorName":"EqualityOperator","status":"Killed","location":{"start":{"line":1,"column":1}}},
  {"mutatorName":"BooleanLiteral","status":"Survived","location":{"start":{"line":2,"column":5}}}]}}}
JSON
cat > "$TMP/clean.json" <<'JSON'
{"go_module":"m","files":[{"file_name":"lib/parser.js","mutations":[
  {"type":"CONDITIONALS_NEGATION","status":"KILLED","line":1,"column":1}]}]}
JSON

repo="$TMP/repo"
mkdir -p "$repo/lib" "$repo/.beads"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
printf '{"id":"task-1","title":"Parser","description":"Parse input","acceptance":["tests pass"]}\n' > .beads/issues.jsonl
printf '{"id":"epic-1","title":"Parsing","description":"Parsing epic","acceptance":["children done"],"children":["task-1"]}\n' >> .beads/issues.jsonl
printf "'use strict';\nmodule.exports = i => JSON.parse(i);\n" > lib/parser.js
git add .
git commit -qm initial
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = i => JSON.parse(i.trim());\n" > lib/parser.js
git add .
git commit -qm tweak

run() {  # run <expected-exit> <args...>; echoes the review path
  local want="$1"; shift
  set +e
  out="$("$TMP/metareview" "$@" 2>"$TMP/err")"
  code=$?
  set -e
  if [ "$code" -ne "$want" ]; then
    echo "expected exit $want, got $code, for: $*" >&2
    cat "$TMP/err" >&2
    exit 1
  fi
  echo "$out" | tail -1
}

# 1. A surviving mutant blocks, and an uncovered site is reported without blocking.
review="$(run 1 review task-done task-1 --base "$base" --mutation-report "$TMP/survivors.json")"
grep -q "NEEDS_REVISION" "$repo/$review"
grep -q "Surviving mutant: INVERT_NEGATIVES" "$repo/$review"
grep -q "Uncovered mutation sites in lib/parser.js" "$repo/$review"
grep -q "mutation-gremlins" "$repo/$review"

# 2. The run gremlins scored 100% and exited 0 on. Undecided mutants are not kills, so this
#    must not pass — and it must be ONE finding about the run, not one per timed-out mutant.
review="$(run 1 review task-done task-1 --base "$base" --mutation-report "$TMP/timeouts.json")"
grep -q "Mutation run did not decide every mutant" "$repo/$review"
grep -q "2 mutant(s) undecided" "$repo/$review"
test "$(grep -c "did not decide every mutant" "$repo/$review")" -eq 1

# 3. The cross-language schema is read without being told which engine wrote it.
review="$(run 1 review task-done task-1 --base "$base" --mutation-report "$TMP/stryker.json")"
grep -q "mutation-stryker" "$repo/$review"
grep -q "Surviving mutant: BooleanLiteral" "$repo/$review"

# 4. A fully-killed run adds nothing at all: no findings, and no "you should mutate more".
#    In its own repository, because findings persist in the ledger by design: the survivors the
#    runs above recorded are still open, and would show up here as somebody else's problem.
#    Asserted as "no difference from the same review without a report" — whether this target
#    passes on its own is not what is being tested.
fresh="$TMP/fresh"
git clone -q "$repo" "$fresh"
cd "$fresh"
git config user.email test-user
git config user.name "Test User"
bare="$(run 1 review task-done task-1 --base "$base")"
bare_findings="$(grep -c "^### " "$fresh/$bare")"
rm -rf .metareview docs/metareview
clean="$(run 1 review task-done task-1 --base "$base" --mutation-report "$TMP/clean.json")"
if grep -q "mutation-" "$fresh/$clean"; then echo "a fully-killed run must add no findings" >&2; exit 1; fi
test "$(grep -c "^### " "$fresh/$clean")" -eq "$bare_findings"
cd "$repo"

# 5. Every review surface accepts the flag, and reports reach all of them.
review="$(run 1 review pr-ready --base "$base" --mutation-report "$TMP/survivors.json")"
grep -q "Surviving mutant" "$repo/$review"
review="$(run 1 review epic-ready epic-1 --base "$base" --mutation-report "$TMP/survivors.json")"
grep -q "Surviving mutant" "$repo/$review"

# 6. Two engines over the same code: the site they agree on is reported once.
run 1 review task-done task-1 --base "$base" \
  --mutation-report "$TMP/survivors.json" --mutation-report "$TMP/survivors.json" > "$TMP/dup.out"
review="$(tail -1 "$TMP/dup.out")"
test "$(grep -c "Surviving mutant: INVERT_NEGATIVES" "$repo/$review")" -eq 1

# 7. A report the gate cannot read is refused at the flag, not treated as a clean run. Silently
#    skipping it would let a typo in a path read exactly like a package with no survivors.
for bad in "$TMP/missing.json" "$TMP"; do
  run 2 review task-done task-1 --base "$base" --mutation-report "$bad" > /dev/null
done
printf 'this is a test log, not a report\n' > "$TMP/notareport.json"
run 2 review task-done task-1 --base "$base" --mutation-report "$TMP/notareport.json" > /dev/null
grep -q "Invalid mutation report" "$TMP/err"

echo "test-mutation-report: ok"
