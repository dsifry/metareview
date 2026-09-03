#!/usr/bin/env bash
set -euo pipefail

# Build B fast-follow: epic-ready REQUIRES an adjudicated adversarial review over its INTEGRATION DIFF (a
# review-evidence marker for scope "epic-ready" matching the exact base..HEAD). The deterministic heuristic
# pass is no longer sufficient on its own. Verified per-scenario in a FRESH repo with a CLEAN roll-up (children
# carry passing task-done logs), so the ONLY blocker is the adversarial requirement. The evidence-free path is
# used deliberately: passing child review logs satisfy the acceptance pre-check without an in-repo evidence
# file that would dirty the tree and trip working-tree-unattested.

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/metareview"
(cd "$ROOT" && go build -o "$BIN" ./cmd/metareview)

verdict() { grep -A2 '## Verdict' "$1" | tail -1 | tr -d '[:space:]'; }

# build_epic_repo <dir>: a fresh repo with a clean epic roll-up (epic-1 -> task-1,task-2 both PASS), on a work
# branch diverged from main by one commit (the integration diff is docs/change.md, a non-generated file).
build_epic_repo() {
  local repo="$1"
  mkdir -p "$repo/.beads" "$repo/docs/metareview/reviews"
  cd "$repo"
  git init -q -b main; git config user.email t@e; git config user.name t
  {
    printf '{"id":"epic-1","title":"Parser epic","description":"Ship a documented parser.","children":["task-1","task-2"]}\n'
    printf '{"id":"task-1","title":"Parser","description":"Implement parser."}\n'
    printf '{"id":"task-2","title":"Tests","description":"Add tests."}\n'
  } > .beads/issues.jsonl
  printf '# metareview: task-done review\n\nRun ID: `mrv-task-1`\n\nTarget: `task-1`\n\n## Verdict\n\nPASS\n' > docs/metareview/reviews/task-1.md
  printf '# metareview: task-done review\n\nRun ID: `mrv-task-2`\n\nTarget: `task-2`\n\n## Verdict\n\nPASS\n' > docs/metareview/reviews/task-2.md
  printf 'initial\n' > docs/change.md
  git add .; git -c commit.gpgsign=false commit -qm base
  git checkout -q -b work
  printf 'updated\n' > docs/change.md; git add docs/change.md; git -c commit.gpgsign=false commit -qm change
}

# mkfsmrun <run-id> <base-sha> : forge an FSM audit whose init matches base..HEAD and reaches a passing outcome.
mkfsmrun() {
  mkdir -p ".metareview/runs/$1"
  { printf '{"type":"init","data":{"base_sha":"%s","head":"%s","workflow":"epic-review-loop"}}\n' "$2" "$(git rev-parse HEAD)";
    printf '{"type":"transition","data":{"from":"adjudicate","to":"done","gate":"confirmed_nonempty","outcome":"reviewed","head":"%s"}}\n' "$(git rev-parse HEAD)";
  } > ".metareview/runs/$1/audit.jsonl"
}

# The gate exits 1 when it blocks; `|| true` keeps that from aborting the pipeline under `set -o pipefail`.
run_gate() { "$BIN" review epic-ready epic-1 --base main 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true; }

# scenario <label> <want-verdict> <marker-cmd|-> [extra-check]
scenario() {
  local label="$1" want="$2" marker="$3" extra="${4:-}"
  local repo; repo="$(mktemp -d)"
  (
    build_epic_repo "$repo"
    if [ "$marker" != "-" ]; then eval "$marker"; fi
    log="$(run_gate)"
    got="$(verdict "$log")"
    if [ "$got" != "$want" ]; then echo "FAIL: [$label] verdict=$got, want $want"; exit 1; fi
    if [ "$label" = "no-marker" ] && ! grep -q 'adversarial-review-reviewer' "$log"; then
      echo "FAIL: [$label] the adversarial-review-reviewer blocker is missing"; exit 1; fi
    if [ -n "$extra" ]; then eval "$extra"; fi
    echo "ok: $label -> $got"
  )
  rm -rf "$repo"
}

rec='"$BIN" review record-lenses --scope epic-ready --base main --lenses epic-integration,acceptance-vs-intent'

# 1. No marker -> the adversarial requirement blocks (NEEDS_REVISION), the heuristics being clean. The
# remediation must steer to epic-review-loop (the epic rubric), NOT the default review-loop (task-done rubric).
scenario "no-marker" "NEEDS_REVISION" "-" \
  'grep -q "epic-review-loop" "$log" || { echo "FAIL: [no-marker] remediation must name epic-review-loop"; exit 1; }
   grep -q -- "--workflow review-loop " "$log" && { echo "FAIL: [no-marker] must not steer epic to review-loop"; exit 1; }; true'

# 2. An in-session-emulated marker satisfies the gate but carries the weaker-evidence advisory note.
scenario "emulated-pass" "PASS_ADVISORY" \
  "$rec --verdict PASS --mode in-session-emulated >/dev/null 2>&1" \
  'grep -q "in-session-emulated" "$log" || { echo "FAIL: [emulated-pass] advisory note missing"; exit 1; }'

# 3. A subagent-adjudicated marker mirroring a real epic-review-loop run passes WITHOUT the advisory note.
scenario "subagent-pass" "PASS" \
  "mkfsmrun fsm-1 \"\$(git rev-parse main)\" && $rec --verdict PASS --mode subagent-adjudicated --from-run fsm-1 >/dev/null 2>&1" \
  'grep -q "in-session-emulated" "$log" && { echo "FAIL: [subagent-pass] must not be advisory"; exit 1; }; true'

# 4. A recorded non-pass verdict blocks on the review's own findings (attributed to the adversarial reviewer).
scenario "marker-findings" "NEEDS_REVISION" \
  "$rec --verdict NEEDS_REVISION --mode in-session-emulated >/dev/null 2>&1" \
  'grep -q "unresolved findings" "$log" || { echo "FAIL: [marker-findings] the unresolved-findings blocker is missing"; exit 1; }'

# 5. Opt-out restores the legacy deterministic pass (a clean roll-up PASSes with no marker).
scenario "flag-opt-out" "PASS" "export METAREVIEW_ALLOW_MECHANICAL_PASS=1"

# 6. Currency: a marker recorded at HEAD does not satisfy a later HEAD (new commit on the epic branch).
repo="$(mktemp -d)"
(
  build_epic_repo "$repo"
  eval "$rec --verdict PASS --mode in-session-emulated" >/dev/null 2>&1
  printf 'more\n' >> docs/change.md; git add docs/change.md; git -c commit.gpgsign=false commit -qm more  # HEAD moves
  got="$(verdict "$(run_gate)")"
  if [ "$got" != "NEEDS_REVISION" ]; then echo "FAIL: [stale-head] verdict=$got, want NEEDS_REVISION"; exit 1; fi
  echo "ok: stale-head -> $got"
)
rm -rf "$repo"

# 7. Dirty working tree is unattested: with a valid marker but an uncommitted change to a reviewed file, the
# gate blocks on working-tree-unattested (epic-ready folds the working tree into the reviewed surface).
repo="$(mktemp -d)"
(
  build_epic_repo "$repo"
  eval "$rec --verdict PASS --mode in-session-emulated" >/dev/null 2>&1
  printf 'uncommitted\n' >> docs/change.md   # dirty a reviewed (non-generated) file, do NOT commit
  log="$(run_gate)"
  got="$(verdict "$log")"
  if [ "$got" != "NEEDS_REVISION" ]; then echo "FAIL: [working-tree] verdict=$got, want NEEDS_REVISION"; exit 1; fi
  grep -q "does not cover the working tree" "$log" || { echo "FAIL: [working-tree] unattested blocker missing"; exit 1; }
  echo "ok: working-tree-unattested -> $got"
)
rm -rf "$repo"

# 8. Base-consistency across an advanced main: main gains a commit after the branch point; driving the gate and
# the recorder with the IDENTICAL explicit --base (the merge-base) still matches. Uses a base SHA, not "main".
repo="$(mktemp -d)"
(
  build_epic_repo "$repo"
  mb="$(git merge-base HEAD main)"
  git checkout -q main; printf 'moved\n' > docs/moved.md; git add docs/moved.md; git -c commit.gpgsign=false commit -qm advance
  git checkout -q work
  "$BIN" review record-lenses --scope epic-ready --base "$mb" --lenses epic-integration --verdict PASS --mode in-session-emulated >/dev/null 2>&1
  log="$("$BIN" review epic-ready epic-1 --base "$mb" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true)"
  got="$(verdict "$log")"
  if [ "$got" != "PASS_ADVISORY" ]; then echo "FAIL: [base-advance] verdict=$got, want PASS_ADVISORY"; exit 1; fi
  echo "ok: base-advance-identical-base -> $got"
)
rm -rf "$repo"

# 9. CLI seam: --scope epic-ready is accepted and round-trips; --scope bogus rejected; and the two build-B
# dogfood bugs (forged independence, wrong diff) must be pinned for epic-ready, not assumed from pr-ready.
repo="$(mktemp -d)"
(
  build_epic_repo "$repo"
  "$BIN" review record-lenses --scope epic-ready --base main --lenses epic-integration --verdict PASS --mode in-session-emulated >/dev/null 2>&1 \
    || { echo "FAIL: --scope epic-ready must be accepted"; exit 1; }
  grep -q '"reviewedScope":"epic-ready"' .metareview/runs.jsonl || { echo "FAIL: epic-ready marker not recorded"; exit 1; }
  reject() { local label="$1"; shift; local s=0; "$BIN" review record-lenses "$@" >/dev/null 2>&1 || s=$?; \
    if [ "$s" -eq 0 ]; then echo "FAIL: [$label] the CLI accepted an invalid marker"; exit 1; fi; }
  ebase="--scope epic-ready --base main --lenses epic-integration"
  # shellcheck disable=SC2086
  reject reject-bogus-scope --scope bogus --base main --lenses epic-integration --mode in-session-emulated
  # shellcheck disable=SC2086
  reject reject-no-fromrun  $ebase --mode subagent-adjudicated
  # A run that reviewed a DIFFERENT diff must not credit an epic-ready subagent marker.
  mkdir -p .metareview/runs/other
  printf '{"type":"init","data":{"base_sha":"deadbeef","head":"cafef00d","workflow":"epic-review-loop"}}\n' > .metareview/runs/other/audit.jsonl
  # shellcheck disable=SC2086
  reject reject-wrong-diff  $ebase --mode subagent-adjudicated --from-run other
  echo "ok: cli-epic-ready-scope-and-rejects"
)
rm -rf "$repo"

echo "test-epic-ready-adjudicated-review: ok"
