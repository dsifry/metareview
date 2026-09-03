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
    echo "ok: $label -> $got"
  )
  rm -rf "$repo" "$ev"
}

rec='"$BIN" review record-lenses --scope pr-ready --base main'

scenario "no-marker"        "NEEDS_REVISION" "-"
scenario "marker-pass"      "PASS_ADVISORY"  "$rec --verdict PASS --mode subagent-adjudicated --lenses security >/dev/null 2>&1"
scenario "marker-findings"  "NEEDS_REVISION" "$rec --verdict NEEDS_REVISION --mode subagent-adjudicated >/dev/null 2>&1"
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
  "$BIN" review record-lenses --scope pr-ready --base main --verdict PASS --mode subagent-adjudicated >/dev/null 2>&1
  printf 'var Y = 2\n' >> f.go; git add f.go; git -c commit.gpgsign=false commit -qm more   # HEAD moves
  log="$("$BIN" review pr-ready --base main --evidence "$ev" 2>/dev/null | grep -oE 'docs/metareview/reviews/[^ ]+\.md' | tail -1 || true)"
  got="$(verdict "$log")"
  if [ "$got" != "NEEDS_REVISION" ]; then echo "FAIL: [stale-head] verdict=$got, want NEEDS_REVISION"; exit 1; fi
  echo "ok: stale-head -> $got"
)
rm -rf "$repo" "$ev"

echo "test-require-adjudicated-review: ok"
