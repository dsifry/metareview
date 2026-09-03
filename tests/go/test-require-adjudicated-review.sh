#!/usr/bin/env bash
set -euo pipefail

# Build B end-to-end: pr-ready REQUIRES an adjudicated lens review (a review-evidence marker matching HEAD).
# The deterministic pass is gone. Verified per-scenario in a FRESH repo (a re-run would otherwise chain its own
# NEEDS_REVISION into the next answer), with the validation evidence written OUTSIDE the repo (an in-repo file
# would dirty the tree and block on its own).

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/metareview"
(cd "$ROOT" && go build -o "$BIN" ./cmd/metareview)

verdict() { # <review.md> -> the verdict word
  grep -A2 '## Verdict' "$1" | tail -1 | tr -d '[:space:]'
}

# scenario <label> <expected-verdict> <marker-cmd|-> [pre-setup-cmd]
scenario() {
  local label="$1" want="$2" marker="$3" pre="${4:-:}"
  local repo ev
  repo="$(mktemp -d)"
  ev="$(mktemp)"
  (
    cd "$repo"
    git init -q -b main
    git config user.email t@e; git config user.name t
    printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
    git checkout -q -b work
    printf 'package p\nvar X = 1\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm change
    "$BIN" evidence run -- true > "$ev" 2>/dev/null
    eval "$pre"  # optional environment setup (e.g. export the opt-out flag)
    if [ "$marker" != "-" ]; then eval "$marker"; fi
    log="$("$BIN" review pr-ready --base main --evidence "$ev" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true)"
    got="$(verdict "$log")"
    if [ "$got" != "$want" ]; then
      echo "FAIL: [$label] verdict=$got, want $want"; exit 1
    fi
    # When required and unsatisfied, the adversarial-review-reviewer row must be the blocker.
    if [ "$label" = "no-marker" ] && ! grep -q 'adversarial-review-reviewer' "$log"; then
      echo "FAIL: [$label] the adversarial-review-reviewer blocker is missing"; exit 1
    fi
    # An in-session-emulated marker passes, but the review MUST carry the weaker-evidence advisory note —
    # otherwise a broken emulated branch would be indistinguishable from an independent PASS.
    if [ "$label" = "emulated-pass" ] && ! grep -q 'in-session-emulated' "$log"; then
      echo "FAIL: [$label] the emulated-review advisory note is missing"; exit 1
    fi
    echo "ok: $label -> $got"
  )
  rm -rf "$repo" "$ev"
}

rec='"$BIN" review record-lenses --scope pr-ready --base main'
# subagent-adjudicated evidence must mirror a real FSM run, so it is admitted only with a --from-run that
# exists on disk. A self-attested review has no such run; this fake run stands in for one.
fakerun='mkdir -p .metareview/runs/fsm-1 && : > .metareview/runs/fsm-1/audit.jsonl'

scenario "no-marker"        "NEEDS_REVISION" "-"
scenario "marker-pass"      "PASS_ADVISORY"  "$fakerun && $rec --verdict PASS --mode subagent-adjudicated --from-run fsm-1 --lenses security >/dev/null 2>&1"
scenario "marker-findings"  "NEEDS_REVISION" "$rec --verdict NEEDS_REVISION --mode in-session-emulated >/dev/null 2>&1"
scenario "emulated-pass"    "PASS_ADVISORY"  "$rec --verdict PASS --mode in-session-emulated >/dev/null 2>&1"
scenario "flag-opt-out"     "PASS_ADVISORY"  "-" 'export METAREVIEW_ALLOW_MECHANICAL_PASS=1'

# A marker for a DIFFERENT head must not satisfy the gate (currency): record at HEAD, then add a commit.
repo="$(mktemp -d)"; ev="$(mktemp)"
(
  cd "$repo"
  git init -q -b main; git config user.email t@e; git config user.name t
  printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
  git checkout -q -b work
  printf 'package p\nvar X = 1\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm change
  "$BIN" evidence run -- true > "$ev" 2>/dev/null
  "$BIN" review record-lenses --scope pr-ready --base main --verdict PASS --mode in-session-emulated >/dev/null 2>&1
  printf 'var Y = 2\n' >> f.go; git add f.go; git -c commit.gpgsign=false commit -qm more   # HEAD moves
  log="$("$BIN" review pr-ready --base main --evidence "$ev" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true)"
  got="$(verdict "$log")"
  if [ "$got" != "NEEDS_REVISION" ]; then echo "FAIL: [stale-head] verdict=$got, want NEEDS_REVISION"; exit 1; fi
  echo "ok: stale-head -> $got"
)
rm -rf "$repo" "$ev"

# A marker for a NARROWER base must not satisfy a WIDER gate (base currency): review over HEAD~1..HEAD, then
# run the gate over main..HEAD (two commits). Same head, different base → the marker must not be credited.
repo="$(mktemp -d)"; ev="$(mktemp)"
(
  cd "$repo"
  git init -q -b main; git config user.email t@e; git config user.name t
  printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
  git checkout -q -b work
  printf 'package p\nvar X = 1\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm change1
  printf 'var Y = 2\n' >> f.go; git add f.go; git -c commit.gpgsign=false commit -qm change2
  "$BIN" evidence run -- true > "$ev" 2>/dev/null
  # Record over the ONE-commit diff (base = HEAD~1). Head is unchanged, but the reviewed base is narrower.
  "$BIN" review record-lenses --scope pr-ready --base HEAD~1 --verdict PASS --mode in-session-emulated >/dev/null 2>&1
  log="$("$BIN" review pr-ready --base main --evidence "$ev" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true)"
  got="$(verdict "$log")"
  if [ "$got" != "NEEDS_REVISION" ]; then echo "FAIL: [stale-base] verdict=$got, want NEEDS_REVISION"; exit 1; fi
  echo "ok: stale-base -> $got"
)
rm -rf "$repo" "$ev"

# The CLI seam must refuse to forge independent evidence: subagent-adjudicated requires a --from-run that
# exists, so it cannot be hand-typed to launder a self-attested review as full-strength.
repo="$(mktemp -d)"
(
  cd "$repo"
  git init -q -b main; git config user.email t@e; git config user.name t
  printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
  status=0
  "$BIN" review record-lenses --scope pr-ready --base main --mode subagent-adjudicated >/dev/null 2>&1 || status=$?
  if [ "$status" -eq 0 ]; then echo "FAIL: [reject-no-fromrun] subagent-adjudicated was accepted without --from-run"; exit 1; fi
  status=0
  "$BIN" review record-lenses --scope pr-ready --base main --mode subagent-adjudicated --from-run nope >/dev/null 2>&1 || status=$?
  if [ "$status" -eq 0 ]; then echo "FAIL: [reject-bad-fromrun] subagent-adjudicated was accepted for a nonexistent run"; exit 1; fi
  status=0
  "$BIN" review record-lenses --scope pr-ready --base main --mode subagent-adjudicated --from-run '../../etc' >/dev/null 2>&1 || status=$?
  if [ "$status" -eq 0 ]; then echo "FAIL: [reject-traversal-fromrun] a path-traversal run id was accepted"; exit 1; fi
  echo "ok: cli-rejects-forged-subagent-adjudicated"
)
rm -rf "$repo"

echo "test-require-adjudicated-review: ok"
