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

# A run id is meaningless without a named lens, and subagent-adjudicated must mirror a real FSM review run:
# --from-run must exist, its init must record the SAME base..head, AND it must reach a PASSING terminal
# transition (clean|reviewed|fixed). mkfsmrun forges such a two-event audit; mkfsmrun_init writes only the
# init (an incomplete run), and mkfsmrun_failed a run that reviewed the diff but came out non-clean.
mkfsmrun_init() { # <run-id> : init line only (matching base..HEAD) — an incomplete run
  mkdir -p ".metareview/runs/$1"
  printf '{"type":"init","data":{"base_sha":"%s","head":"%s","workflow":"review-loop"}}\n' \
    "$(git rev-parse main)" "$(git rev-parse HEAD)" > ".metareview/runs/$1/audit.jsonl"
}
mkfsmrun() { # <run-id> : init + a PASSING terminal transition
  mkfsmrun_init "$1"
  printf '{"type":"transition","data":{"from":"adjudicate","to":"done","gate":"confirmed_nonempty","outcome":"reviewed","head":"%s"}}\n' \
    "$(git rev-parse HEAD)" >> ".metareview/runs/$1/audit.jsonl"
}
mkfsmrun_failed() { # <run-id> : init + a NON-passing terminal transition
  mkfsmrun_init "$1"
  printf '{"type":"transition","data":{"from":"verify","to":"done","gate":"stuck","outcome":"failed","head":"%s"}}\n' \
    "$(git rev-parse HEAD)" >> ".metareview/runs/$1/audit.jsonl"
}
rec='"$BIN" review record-lenses --scope pr-ready --base main --lenses security'

scenario "no-marker"        "NEEDS_REVISION" "-"
scenario "marker-pass"      "PASS_ADVISORY"  "mkfsmrun fsm-1 && $rec --verdict PASS --mode subagent-adjudicated --from-run fsm-1 >/dev/null 2>&1"
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
  "$BIN" review record-lenses --scope pr-ready --base main --lenses security --verdict PASS --mode in-session-emulated >/dev/null 2>&1
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
  "$BIN" review record-lenses --scope pr-ready --base HEAD~1 --lenses security --verdict PASS --mode in-session-emulated >/dev/null 2>&1
  log="$("$BIN" review pr-ready --base main --evidence "$ev" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true)"
  got="$(verdict "$log")"
  if [ "$got" != "NEEDS_REVISION" ]; then echo "FAIL: [stale-base] verdict=$got, want NEEDS_REVISION"; exit 1; fi
  echo "ok: stale-base -> $got"
)
rm -rf "$repo" "$ev"

# The CLI seam must refuse to forge independent evidence and refuse a lens-less marker.
repo="$(mktemp -d)"
(
  cd "$repo"
  git init -q -b main; git config user.email t@e; git config user.name t
  printf 'package p\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm base
  git checkout -q -b work
  printf 'package p\nvar X = 1\n' > f.go; git add f.go; git -c commit.gpgsign=false commit -qm change
  reject() { # <label> <args...> : the record-lenses call must be REFUSED (nonzero exit)
    local label="$1"; shift
    local status=0
    "$BIN" review record-lenses "$@" >/dev/null 2>&1 || status=$?
    if [ "$status" -eq 0 ]; then echo "FAIL: [$label] the CLI accepted an invalid marker"; exit 1; fi
  }
  base="--scope pr-ready --base main --lenses security"
  # subagent-adjudicated cannot be hand-typed without a real, matching FSM run:
  # shellcheck disable=SC2086
  reject reject-no-fromrun  $base --mode subagent-adjudicated
  # shellcheck disable=SC2086
  reject reject-bad-fromrun $base --mode subagent-adjudicated --from-run nope
  # shellcheck disable=SC2086
  reject reject-traversal   $base --mode subagent-adjudicated --from-run '../../etc'
  # A run whose audit.jsonl is empty (a bare directory, no real init) is rejected.
  mkdir -p .metareview/runs/empty; : > .metareview/runs/empty/audit.jsonl
  # shellcheck disable=SC2086
  reject reject-empty-audit $base --mode subagent-adjudicated --from-run empty
  # A run that exists but reviewed a DIFFERENT diff (init head does not match) is rejected.
  mkdir -p .metareview/runs/other
  printf '{"type":"init","data":{"base_sha":"deadbeef","head":"cafef00d","workflow":"review-loop"}}\n' > .metareview/runs/other/audit.jsonl
  # shellcheck disable=SC2086
  reject reject-wrong-diff  $base --mode subagent-adjudicated --from-run other
  # A run over the RIGHT diff that never reached a passing terminal transition (incomplete) is rejected.
  mkfsmrun_init incomplete
  # shellcheck disable=SC2086
  reject reject-incomplete  $base --mode subagent-adjudicated --from-run incomplete
  # A run over the right diff whose terminal outcome was non-clean (failed) is rejected.
  mkfsmrun_failed failedrun
  # shellcheck disable=SC2086
  reject reject-failed      $base --mode subagent-adjudicated --from-run failedrun
  # A marker with no named lens is refused (it would let the gate pass on nothing).
  reject reject-no-lenses   --scope pr-ready --base main --mode in-session-emulated
  echo "ok: cli-rejects-invalid-markers"
)
rm -rf "$repo"

echo "test-require-adjudicated-review: ok"
