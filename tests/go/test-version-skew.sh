#!/usr/bin/env bash
# AC-5.4 (#180): a run written by this binary, read by a binary built from the previous release tag, fails with a
# specific error — never a panic, never a verdict. Both binaries are built here, so CI runs the check on every push.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
PREV_TAG="${METAREVIEW_PREV_TAG:-v0.12.0}"
if ! git rev-parse -q --verify "refs/tags/$PREV_TAG" >/dev/null; then
  # In CI the tag must be there (the checkout fetches full history): a missing tag there is a failure, never a pass.
  if [ -n "${CI:-}" ]; then echo "FAIL: $PREV_TAG is not in this clone" >&2; exit 1; fi
  echo "test-version-skew: skipped ($PREV_TAG is not in this clone; fetch tags to run it)"
  exit 0
fi

WORK="$(cd "$(mktemp -d)" && pwd -P)"
cleanup() { git -C "$ROOT" worktree remove --force "$WORK/prev-src" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT
go build -o "$WORK/new" ./cmd/metareview
git worktree add -q --detach "$WORK/prev-src" "$PREV_TAG"
(cd "$WORK/prev-src" && GOFLAGS="" go build -o "$WORK/prev" ./cmd/metareview)
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@x GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@x
unset MOCK_AI MRV_RUN_ID

field() { node -e 'const o=JSON.parse(require("fs").readFileSync(0,"utf8"));const v=process.argv[1].split(".").reduce((a,k)=>a==null?a:a[k],o);process.stdout.write(v===undefined?"<absent>":(typeof v==="object"?JSON.stringify(v):String(v)))' "$1"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

REPO="$WORK/repo"; mkdir -p "$REPO"; cd "$REPO"
git init -q -b main
printf 'package f\n' > f.go
mkdir -p scenarios && cp -R "$ROOT/testdata/fsm/scenarios/." scenarios/
printf 'scenarios/\n.metareview/\n' > .gitignore
git add -A && git commit -q -m base
printf 'package f\n\n// change\n' > f.go && git add f.go && git commit -q -m change
ID="$("$WORK/new" fsm init --workflow sdlc-loop --var JUDGE=gpt-5.2 --var JUDGE_EFFORT=medium --mock-ai scenarios/sdlc-loop/happy --base HEAD~1 | field run_id)"
STORE="$(git rev-parse --path-format=absolute --git-common-dir)/metareview/runs"
grep -q "\"writer\":" "$STORE/$ID/audit.jsonl" || fail "the new binary records its version in the run"

# read_with_prev: the previous binary reads the run; it must fail with a JSON error code, not panic or report a state.
read_with_prev() {
  local out code
  set +e; out="$("$WORK/prev" fsm state --run "$ID" 2>"$WORK/prev.err")"; code=$?; set -e
  [ "$code" != 0 ] || fail "the previous binary read a newer run as if it were its own: $out"
  grep -q 'panic' "$WORK/prev.err" && fail "the previous binary panicked: $(cat "$WORK/prev.err")"
  [ "$(printf '%s' "$out" | field ok)" = false ] || fail "not a JSON error: $out"
  printf '%s' "$out" | field code
}
# From its own store location the previous release may not find the run at all (0.12 kept runs in the main
# checkout's .metareview/runs); copied there, the run's init no longer decodes: its strict payload decoder refuses
# the new `writer` field. Either way a specific error, never a verdict.
got="$(read_with_prev)"
case "$got" in ERR_RUN_NOT_FOUND|ERR_AUDIT_INVALID|ERR_AUDIT_VERSION) ;; *) fail "unexpected error from the previous binary: $got" ;; esac
mkdir -p .metareview/runs && cp -R "$STORE/$ID" .metareview/runs/
got="$(read_with_prev)"
case "$got" in ERR_AUDIT_INVALID|ERR_AUDIT_VERSION) ;; *) fail "the previous binary must refuse the run's audit, got: $got" ;; esac

# This binary's side, end to end: a run written by a newer minor release (this tree built as 99.0.0) is refused
# with ERR_AUDIT_VERSION — never folded, never misread.
go build -C "$ROOT" -ldflags "-X github.com/dsifry/metareview/internal/fsm/run.ReaderVersion=99.0.0" -o "$WORK/future" ./cmd/metareview
FUTURE="$("$WORK/future" fsm init --workflow sdlc-loop --var JUDGE=gpt-5.2 --var JUDGE_EFFORT=medium --mock-ai scenarios/sdlc-loop/happy --base HEAD~1 | field run_id)"
grep -q '"writer":"99.0.0"' "$STORE/$FUTURE/audit.jsonl" || fail "the future binary must record its version"
set +e; out="$("$WORK/new" fsm state --run "$FUTURE" 2>/dev/null)"; set -e
[ "$(printf '%s' "$out" | field code)" = ERR_AUDIT_VERSION ] || fail "a run from a newer writer must be ERR_AUDIT_VERSION: $out"

echo "test-version-skew: ok"
