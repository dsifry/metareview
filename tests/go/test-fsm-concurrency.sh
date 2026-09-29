#!/usr/bin/env bash
# Black-box suite for #180: many agents running metareview at once, in separate worktrees and in one.
#   AC-5.1  five fix loops in five worktrees, concurrently, each to completion, with verifying audit chains
#   AC-5.2  a second fix loop in the same worktree fails fast naming the holder; the first completes intact
#   AC-5.3  a stale edit lock (its holder gone) is taken over; a live one never is
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
go build -o "$ROOT/bin/metareview" ./cmd/metareview
MRV="$ROOT/bin/metareview"

WORK="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$WORK"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@x GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@x
unset MOCK_AI MRV_RUN_ID ANTHROPIC_API_KEY OPENAI_API_KEY ANTHROPIC_BASE_URL OPENAI_BASE_URL
VARS=(--var JUDGE=gpt-5.2 --var JUDGE_EFFORT=medium)
FINDINGS='{"findings":[{"tag":"bug","file":"f.go","start_line":2,"end_line":2,"issue":"nil deref in f.go","consequence":"panics when the flag is unset","confidence":75,"severity":"P1"}]}'

field() { node -e 'const o=JSON.parse(require("fs").readFileSync(0,"utf8"));const v=process.argv[1].split(".").reduce((a,k)=>a==null?a:a[k],o);process.stdout.write(v===undefined?"<absent>":(typeof v==="object"?JSON.stringify(v):String(v)))' "$1"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# The repository: a base commit and a reviewable change, the mock scenarios in the main checkout (a scenario must
# live inside RepoRoot, the main worktree, which every linked worktree shares).
REPO="$WORK/repo"; mkdir -p "$REPO"; cd "$REPO"
git init -q -b main
printf 'package f\n' > f.go
mkdir -p scenarios && cp -R "$ROOT/testdata/fsm/scenarios/." scenarios/
printf 'scenarios/\nfixtures/\n.metareview/\n' > .gitignore
git add -A && git commit -q -m base
printf 'package f\n\n// reviewable change\n' > f.go && git add f.go && git commit -q -m "reviewable change"
SCENARIO="$REPO/scenarios/sdlc-loop/happy"

# mrv <tag> <args…>: run the CLI; a failure names the command, its output and its stderr.
mrv() {
  local tag="$1" out code; shift
  set +e; out="$($MRV "$@" 2>"$WORK/err-$tag")"; code=$?; set -e
  if [ "$code" != 0 ] && [ "$code" != 3 ]; then fail "$tag: metareview $* exited $code: $out $(cat "$WORK/err-$tag")"; fi
  printf '%s' "$out"
}

# fix_loop <dir> <tag>: one complete mock sdlc-loop-clean in <dir> — discover, adjudicate, fix (a real commit),
# recheck clean — printing its run id. Any unexpected status is a failure.
fix_loop() {
  local dir="$1" tag="$2" out id status
  cd "$dir"
  mkdir -p fixtures && printf '%s' "$FINDINGS" > fixtures/findings.json && printf '{"findings":[]}' > fixtures/empty.json
  out="$(mrv "$tag" fsm init --workflow sdlc-loop-clean "${VARS[@]}" --mock-ai "$SCENARIO" --base HEAD~1)"
  id="$(printf '%s' "$out" | field run_id)"
  local discovered=0
  for _ in $(seq 1 30); do
    out="$(mrv "$tag" fsm advance --run "$id")"
    status="$(printf '%s' "$out" | field status)"
    case "$status" in
      DONE) printf '%s\n' "$id"; return 0 ;;
      ADVANCED) ;;
      NEEDS_INPUT)
        case "$(printf '%s' "$out" | field state)" in
          discover)
            if [ "$discovered" = 0 ]; then data=fixtures/findings.json; discovered=1; else data=fixtures/empty.json; fi
            mrv "$tag" fsm record node-output --run "$id" --node discover --data "$data" >/dev/null ;;
          fix)
            printf '// fixed by %s\n' "$tag" >> f.go && git add f.go && git commit -q -m "fix $tag"
            printf '{"commit":"%s","summary":"fix %s"}' "$(git rev-parse HEAD)" "$tag" > fixtures/fix.json
            mrv "$tag" fsm record node-output --run "$id" --node fix --data fixtures/fix.json >/dev/null ;;
          recheck) mrv "$tag" fsm record node-output --run "$id" --node recheck --data fixtures/empty.json >/dev/null ;;
          *) fail "$tag: unexpected input for $(printf '%s' "$out" | field state): $out" ;;
        esac ;;
      *) fail "$tag: unexpected status: $out" ;;
    esac
  done
  fail "$tag: the loop did not finish"
}

# ---- AC-5.1: five worktrees, five concurrent fix loops ----------------------------------------------------
for i in 1 2 3 4 5; do git -C "$REPO" worktree add -q -b "agent-$i" "$WORK/wt-$i" main; done
pids=()
for i in 1 2 3 4 5; do (fix_loop "$WORK/wt-$i" "agent-$i" > "$WORK/id-$i") & pids+=($!); done
for p in "${pids[@]}"; do wait "$p" || fail "a concurrent fix loop failed"; done
for i in 1 2 3 4 5; do
  id="$(cat "$WORK/id-$i")"; [ -n "$id" ] || fail "worktree $i recorded no run"
  out="$(cd "$WORK/wt-$i" && $MRV fsm state --run "$id")"
  [ "$(printf '%s' "$out" | field outcome)" = clean ] || fail "worktree $i: $out"
  # Each run's own fix, on its own branch, and nothing from another worktree.
  [ "$(git -C "$WORK/wt-$i" log -1 --format=%s)" = "fix agent-$i" ] || fail "worktree $i: head is not its own fix"
  [ -z "$(git -C "$WORK/wt-$i" status --porcelain)" ] || fail "worktree $i: tree not clean"
  [ ! -e "$(git -C "$WORK/wt-$i" rev-parse --path-format=absolute --git-dir)/metareview/edit.lock" ] || fail "worktree $i: lock left behind"
done
# Every run is in the one shared store, and every audit chain folds (fsm state refuses a broken chain).
[ "$(find "$(git -C "$REPO" rev-parse --path-format=absolute --git-common-dir)/metareview/runs" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')" = 5 ] || fail "want 5 runs in the store"

# ---- AC-5.2: two fix loops in one worktree ----------------------------------------------------------------
WT="$WORK/wt-1"; cd "$WT"
git reset -q --hard main
start_to_fix() { # start_to_fix: a fresh run advanced to its fix node's input; prints its id
  local out id
  out="$($MRV fsm init --workflow sdlc-loop-clean "${VARS[@]}" --mock-ai "$SCENARIO" --base HEAD~1)"; id="$(printf '%s' "$out" | field run_id)"
  $MRV fsm advance --run "$id" >/dev/null 2>&1 || true
  $MRV fsm record node-output --run "$id" --node discover --data fixtures/findings.json >/dev/null
  $MRV fsm advance --run "$id" >/dev/null   # → adjudicate
  printf '%s\n' "$id"
}
A="$(start_to_fix)"
$MRV fsm advance --run "$A" > "$WORK/a.json"   # adjudicate → fix: A takes the lock
[ "$(field to < "$WORK/a.json")" = fix ] || fail "A did not reach fix: $(cat "$WORK/a.json")"
B="$(start_to_fix)"
set +e; $MRV fsm advance --run "$B" > "$WORK/b.json" 2>/dev/null; code=$?; set -e
[ "$code" != 0 ] || fail "B entered fix while A held the lock"
[ "$(field code < "$WORK/b.json")" = ERR_EDIT_LOCKED ] || fail "B: want ERR_EDIT_LOCKED: $(cat "$WORK/b.json")"
grep -q "$A" "$WORK/b.json" || fail "B's error must name the holder $A"
[ "$($MRV fsm state --run "$B" | field state)" = adjudicate ] || fail "B must stay at adjudicate"
# A completes, with its diff intact.
$MRV fsm advance --run "$A" >/dev/null 2>&1 || true   # fix input
printf '// fixed by A\n' >> f.go && git add f.go && git commit -q -m "fix A"
printf '{"commit":"%s","summary":"fix A"}' "$(git rev-parse HEAD)" > fixtures/fix.json
$MRV fsm record node-output --run "$A" --node fix --data fixtures/fix.json >/dev/null
$MRV fsm advance --run "$A" >/dev/null   # → recheck: the lock goes
git diff HEAD~1 -- f.go | grep -q '^+// fixed by A' || fail "A's fix diff is not intact"
# Now B goes ahead.
$MRV fsm advance --run "$B" > "$WORK/b.json"
[ "$(field to < "$WORK/b.json")" = fix ] || fail "B must reach fix once A left it: $(cat "$WORK/b.json")"

# ---- AC-5.3: stale and live locks -------------------------------------------------------------------------
LOCK="$(git rev-parse --path-format=absolute --git-dir)/metareview/edit.lock"
[ "$(node -e 'process.stdout.write(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).run_id)' "$LOCK")" = "$B" ] || fail "B holds the lock"
# A live hold is never stolen: C cannot enter fix while B is in its fix node.
C="$(start_to_fix)"
set +e; $MRV fsm advance --run "$C" > "$WORK/c.json" 2>/dev/null; set -e
[ "$(field code < "$WORK/c.json")" = ERR_EDIT_LOCKED ] || fail "a live lock must never be stolen: $(cat "$WORK/c.json")"
# A stale hold — its holder gone (a run that no longer exists) — is taken over.
printf '{"run_id":"mrv-20260101-000000000000000-fsm-gone-gone-00000000","since":"2026-01-01T00:00:00Z"}\n' > "$LOCK"
$MRV fsm advance --run "$C" > "$WORK/c.json"
[ "$(field to < "$WORK/c.json")" = fix ] || fail "a stale lock must be taken over: $(cat "$WORK/c.json")"
[ "$(node -e 'process.stdout.write(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).run_id)' "$LOCK")" = "$C" ] || fail "C holds the lock"

echo "test-fsm-concurrency: ok"
