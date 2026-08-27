#!/usr/bin/env bash
# End-to-end sharded review: plan, results, a fix round, the negative cases, and
# reproducibility. The fixture is a deterministic ~300 KB lint-clean branch diff
# with one file over the shard budget.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)
BIN="$TMP/metareview"
EVIDENCE="$TMP/evidence.json"
cat > "$EVIDENCE" <<'JSON'
{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}
JSON

fail() { echo "test-sharded-review: FAIL: $*" >&2; exit 1; }

filler() { # $1 = seed, $2 = line count
  local seed="$1" count="$2" i
  for ((i = 0; i < count; i++)); do
    printf '%s %04d 0123456789abcdef 0123456789abcdef 0123456789abcdef\n' "$seed" "$i"
  done
}

init_repo() { # $1 = repo dir
  local repo="$1"
  mkdir -p "$repo/src"
  cd "$repo"
  git init -q -b main
  git config user.email test-user
  git config user.name "Test User"
  printf 'seed\n' > README.txt
  git add .
  git commit -qm initial
  git checkout -q -b feature
  local i
  for i in 0 1 2 3 4 5; do
    filler "s$i" 700 > "src/file$i.txt"
  done
  # One file over the 60 KB budget, so the plan chunks it.
  filler "big" 1400 > src/big.txt
  # A determinate marker for the fix round: the replacement is the same length.
  printf 'MARKER AAAA\n' >> src/file2.txt
  git add -A src README.txt
  git commit -qm "big branch change"
}

plan_json() { # $1 = repo, $2 = scope
  local found
  found="$(find "$1/.metareview/shards/$2" -name plan.json 2>/dev/null | head -1)"
  [ -n "$found" ] || fail "no plan.json written for $2"
  printf '%s' "$found"
}

# write_results REPO PLAN [ONLY-CSV] [SKIP_CROSS]
write_results() {
  REPO="$1" PLAN="$2" ONLY="${3:-}" SKIP_CROSS="${4:-}" node - <<'NODE'
const fs = require('fs'), path = require('path');
const plan = JSON.parse(fs.readFileSync(process.env.PLAN, 'utf8'));
const dir = path.join(process.env.REPO, plan.resultsDir);
fs.mkdirSync(dir, { recursive: true });
const only = (process.env.ONLY || '').split(',').filter(Boolean);
const base = { schemaVersion: 1, verdict: 'PASS', reviewer: 'test-shard-reviewer',
  reviewedAt: '2026-08-27T10:00:00Z', findings: [], blockingCount: 0 };
for (const shard of plan.shards) {
  if (only.length && !only.includes(shard.shardId)) continue;
  const result = Object.assign({}, base, {
    id: 'r-' + shard.shardId, kind: 'shard', shardId: shard.shardId,
    shardHash: shard.shardHash, planHash: plan.planHash,
    evidence: [{ note: 'reviewed the ' + shard.shardId + ' pack in full' }],
  });
  fs.writeFileSync(path.join(dir, `${shard.shardId}.${shard.shardHash}.result.json`), JSON.stringify(result));
}
if (plan.shards.length > 1 && process.env.SKIP_CROSS !== '1') {
  const cross = Object.assign({}, base, { id: 'r-cross', kind: 'cross-shard', planHash: plan.planHash,
    evidence: [{ note: 'reviewed the integration seams across every shard' }] });
  fs.writeFileSync(path.join(dir, `cross-shard.${plan.planHash}.result.json`), JSON.stringify(cross));
}
NODE
}

plan_field() { PLAN="$1" FIELD="$2" node -e '
const plan = JSON.parse(require("fs").readFileSync(process.env.PLAN, "utf8"));
process.stdout.write(String(process.env.FIELD === "count" ? plan.shards.length : plan[process.env.FIELD]));
'; }

shard_ids() { PLAN="$1" node -e '
const plan = JSON.parse(require("fs").readFileSync(process.env.PLAN, "utf8"));
process.stdout.write(plan.shards.map(s => s.shardId).join("\n"));
'; }

section_count() { # $1 = log, $2 = heading: bullets under a heading
  LOG="$1" HEADING="$2" node -e '
const fs = require("fs");
const lines = fs.readFileSync(process.env.LOG, "utf8").split("\n");
const start = lines.findIndex(l => l.trim() === process.env.HEADING);
if (start < 0) { process.stdout.write("0"); process.exit(0); }
let n = 0;
for (let i = start + 1; i < lines.length; i++) {
  const line = lines[i];
  if (line.startsWith("#")) break;
  if (line.startsWith("- ")) n++;
}
process.stdout.write(String(n));
'; }

run_review() { # $1 = repo, rest = args; prints the review path, tolerates exit 1
  local repo="$1"; shift
  local out status=0
  out="$(cd "$repo" && "$BIN" "$@" 2>/dev/null)" || status=$?
  if [ "$status" -gt 1 ]; then fail "review exited $status"; fi
  printf '%s' "$out"
}

verdict_of() { # $1 = repo, $2 = review rel path
  sed -n '/^## Verdict$/,$p' "$1/$2" | sed -n '3p'
}

########################################################################
# pr-ready
########################################################################

repo="$TMP/pr"
init_repo "$repo"

log1="$(run_review "$repo" review pr-ready --base main --evidence "$EVIDENCE" --max-attempts 12)"
[ -n "$log1" ] || fail "pr-ready run 1 produced no review log"
[ "$(verdict_of "$repo" "$log1")" = "NEEDS_REVISION" ] || fail "run 1 verdict: $(verdict_of "$repo" "$log1")"
run1_id="$(sed -n 's/^Run ID: `\(.*\)`$/\1/p' "$repo/$log1" | head -1)"

plan="$(plan_json "$repo" pr-ready)"
shards="$(plan_field "$plan" count)"
[ "$shards" -gt 1 ] || fail "fixture produced $shards shard(s); the test needs a multi-shard plan"
[ "$shards" -le 9 ] || fail "fixture produced $shards shards; the blocker list is capped at ten"
grep -q "missing cross-shard result" "$repo/$log1" || fail "run 1 did not report the missing cross-shard result"
while read -r id; do
  grep -q "missing shard result for $id" "$repo/$log1" || fail "run 1 did not name $id"
done <<< "$(shard_ids "$plan")"
for id in $(shard_ids "$plan"); do
  [ -f "$(dirname "$plan")/$id.md" ] || fail "pack for $id was not written"
done
[ -f "$(dirname "$plan")/cross-shard.md" ] || fail "cross-shard pack was not written"

# Results for every shard plus the seams.
write_results "$repo" "$plan"
log2="$(run_review "$repo" review pr-ready --base main --evidence "$EVIDENCE" --previous-run "$run1_id")"
[ "$(verdict_of "$repo" "$log2")" = "PASS_ADVISORY" ] || {
  sed -n '1,80p' "$repo/$log2" >&2
  fail "run 2 verdict: $(verdict_of "$repo" "$log2")"
}
grep -q "## Sharded Review" "$repo/$log2" || fail "run 2 log has no sharded section"
grep -q "Context risk covered by shard reviews" "$repo/$log2" || fail "run 2 did not record the covered advisory"
grep -q "src/big.txt" "$repo/$log2" || fail "run 2 log does not list the chunked file"
grep -q "Files reviewed as chunks" "$repo/$log2" || fail "run 2 log has no chunked-file listing"
node -e '
const fs = require("fs");
const rows = fs.readFileSync(process.argv[1], "utf8").trim().split("\n").map(JSON.parse);
const row = rows.filter(r => r.fingerprint === "pr:architecture:context-risk").pop();
if (!row) { console.error("no pr context-risk row"); process.exit(1); }
if (row.status !== "fixed") { console.error("context-risk row status = " + row.status); process.exit(1); }
' "$repo/.metareview/findings.jsonl" || fail "the context-risk row was not closed"
run2_id="$(sed -n 's/^Run ID: `\(.*\)`$/\1/p' "$repo/$log2" | head -1)"

# Fix round: a determinate same-size edit re-cuts exactly one shard.
(cd "$repo" && sed -i.bak 's/MARKER AAAA/MARKER BBBB/' src/file2.txt && rm -f src/file2.txt.bak &&
  git add -A src && git commit -qm "same-size edit")
log3="$(run_review "$repo" review pr-ready --base main --evidence "$EVIDENCE" --previous-run "$run2_id")"
[ "$(verdict_of "$repo" "$log3")" = "NEEDS_REVISION" ] || fail "run 3 verdict: $(verdict_of "$repo" "$log3")"
ignored="$(section_count "$repo/$log3" "### Ignored result files")"
[ "$ignored" -eq 2 ] || { sed -n '/## Sharded Review/,/## Reviewer/p' "$repo/$log3" >&2;
  fail "run 3 ignored $ignored files, want 2 (one shard plus the cross-shard)"; }
carried="$(grep -c '^| `shard-' "$repo/$log3" || true)"
[ "$carried" -eq "$((shards - 1))" ] || fail "run 3 carried $carried fresh results, want $((shards - 1))"
run3_id="$(sed -n 's/^Run ID: `\(.*\)`$/\1/p' "$repo/$log3" | head -1)"

plan="$(plan_json "$repo" pr-ready)"
write_results "$repo" "$plan"
log4="$(run_review "$repo" review pr-ready --base main --evidence "$EVIDENCE" --previous-run "$run3_id")"
[ "$(verdict_of "$repo" "$log4")" = "PASS_ADVISORY" ] || fail "run 4 verdict: $(verdict_of "$repo" "$log4")"
results_dir="$repo/$(plan_field "$plan" resultsDir)"
[ "$(find "$results_dir" -type f | wc -l | tr -d ' ')" -eq "$((shards + 1))" ] ||
  fail "superseded result files were not collected: $(find "$results_dir" -type f)"
run4_id="$(sed -n 's/^Run ID: `\(.*\)`$/\1/p' "$repo/$log4" | head -1)"

# A result for another plan is ignored, never a blocker.
cat > "$results_dir/shard-0.00112233445566ff.result.json" <<'JSON'
{"schemaVersion":1,"id":"r-other","kind":"shard","shardId":"shard-0","shardHash":"00112233445566ff",
 "planHash":"8899aabbccddeeff","verdict":"PASS","reviewer":"test","reviewedAt":"2026-08-27T10:00:00Z",
 "evidence":[{"note":"a result for a plan that no longer exists"}],"blockingCount":0}
JSON
log5="$(run_review "$repo" review pr-ready --base main --evidence "$EVIDENCE" --previous-run "$run4_id")"
[ "$(verdict_of "$repo" "$log5")" = "PASS_ADVISORY" ] || fail "run 5 verdict: $(verdict_of "$repo" "$log5")"
[ "$(section_count "$repo/$log5" "### Ignored result files")" -eq 1 ] || fail "the other plan's result was not ignored"
[ ! -f "$results_dir/shard-0.00112233445566ff.result.json" ] || fail "the ignored result was not collected"

# A bad --shard-result exits 2 with nothing written.
before="$(shasum "$repo/.metareview/runs.jsonl" | cut -d' ' -f1)"
status=0
(cd "$repo" && "$BIN" review pr-ready --base main --shard-result "$repo/does-not-exist.json" >/dev/null 2>&1) || status=$?
[ "$status" -eq 2 ] || fail "a missing --shard-result exited $status, want 2"
[ "$(shasum "$repo/.metareview/runs.jsonl" | cut -d' ' -f1)" = "$before" ] || fail "a rejected flag still wrote runs.jsonl"

# A plan has one cross-shard result, so a repeated --cross-shard-result exits 2
# rather than letting the last file win silently.
cross="$results_dir/cross-shard.$(plan_field "$(plan_json "$repo" pr-ready)" planHash).result.json"
status=0
(cd "$repo" && "$BIN" review pr-ready --base main \
  --cross-shard-result "$cross" --cross-shard-result "$cross" >/dev/null 2>&1) || status=$?
[ "$status" -eq 2 ] || fail "a repeated --cross-shard-result exited $status, want 2"
[ "$(shasum "$repo/.metareview/runs.jsonl" | cut -d' ' -f1)" = "$before" ] || fail "a repeated flag still wrote runs.jsonl"

# Two runs on unchanged content: one plan hash, byte-identical packs.
pack_dir="$(dirname "$(plan_json "$repo" pr-ready)")"
before_hashes="$(cd "$pack_dir" && shasum ./* | sort)"
run_review "$repo" review pr-ready --base main --evidence "$EVIDENCE" --previous-run "$run4_id" >/dev/null
[ "$(find "$repo/.metareview/shards/pr-ready" -name plan.json | wc -l | tr -d ' ')" -eq 1 ] ||
  fail "a second run on unchanged content produced a second plan"
[ "$(cd "$pack_dir" && shasum ./* | sort)" = "$before_hashes" ] || fail "packs are not byte-reproducible"

########################################################################
# task-done
########################################################################

repo="$TMP/task"
init_repo "$repo"
mkdir -p "$repo/docs/tasks"
cat > "$repo/docs/tasks/task-1.md" <<'TASK'
# task-1: sharded review fixture

Acceptance: the branch diff is reviewed shard by shard.
TASK
(cd "$repo" && git add docs/tasks && git commit -qm "task file")
# Local content is in no pack: staged work still blocks, and an untracked file
# must stay under 4,000 bytes or it raises a reason no shard result can satisfy.
# The lint pattern is assembled here so this file does not carry it literally.
printf 'value := %s(untrusted)\n' eval > "$repo/src/staged.txt"
(cd "$repo" && git add src/staged.txt)
filler "u" 40 > "$repo/untracked.txt"

log1="$(run_review "$repo" review task-done docs/tasks/task-1.md --base main --evidence "$EVIDENCE" --max-attempts 12)"
[ "$(verdict_of "$repo" "$log1")" = "NEEDS_REVISION" ] || fail "task-done run 1 verdict"
run1_id="$(sed -n 's/^Run ID: `\(.*\)`$/\1/p' "$repo/$log1" | head -1)"
plan="$(plan_json "$repo" task-done)"
write_results "$repo" "$plan"

log2="$(run_review "$repo" review task-done docs/tasks/task-1.md --base main --evidence "$EVIDENCE" --previous-run "$run1_id")"
grep -q "## Sharded Review" "$repo/$log2" || fail "task-done log has no sharded section"
grep -q "Context risk covered by shard reviews" "$repo/$log2" || fail "task-done did not record the covered advisory"
grep -q "Unsafe eval introduced" "$repo/$log2" || fail "the staged eval must still block on the satisfied path"
[ "$(verdict_of "$repo" "$log2")" = "NEEDS_REVISION" ] || fail "task-done run 2 verdict: $(verdict_of "$repo" "$log2")"

rm "$repo/src/staged.txt"
(cd "$repo" && git rm -q --cached src/staged.txt)
run2_id="$(sed -n 's/^Run ID: `\(.*\)`$/\1/p' "$repo/$log2" | head -1)"
log3="$(run_review "$repo" review task-done docs/tasks/task-1.md --base main --evidence "$EVIDENCE" --previous-run "$run2_id")"
[ "$(verdict_of "$repo" "$log3")" = "PASS_ADVISORY" ] || {
  sed -n '1,80p' "$repo/$log3" >&2
  fail "task-done run 3 verdict: $(verdict_of "$repo" "$log3")"
}

echo "sharded review: ok"
