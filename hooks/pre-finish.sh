#!/usr/bin/env bash
# hooks/pre-finish.sh — refuse to finish while metareview has unresolved blockers.
#
# This is the thin shim the enforcement model depends on. metareview's gates cannot be routed
# around ONCE A RUN IS INSIDE THEM, because a transition is a machine-evaluated predicate rather
# than an instruction to an agent. But entering them was still policy: nothing stopped a host
# from simply not running metareview. This closes that, by making the host's own "am I done"
# moment consult the contract.
#
# It is deliberately small. All the judgement lives in `metareview status --json`, which returns
# exit 1 when something must be cleared; a hook that re-derived "blocked" would be a second
# definition drifting from the first, which is the failure this repository keeps finding.
#
# Works under Claude Code, Codex and Gemini: each sets a different plugin-root variable, and the
# hook only needs the repository it is standing in.
set -uo pipefail

# Scope. The default is the branch: blockers against its own commits, plus files it changed that
# no passing review has read. Unscoped, `blocked` spans the entire review history — 73 entries
# here, the oldest from 2026-05 — so the hook refuses every session for work it never touched.
# That is a livelock, and a gate an operator has to disable is not a gate.
#
# This defaulted to UNSCOPED while --scope branch already existed, which made the hook unusable
# for the whole time it looked finished: nothing in the tree sets METAREVIEW_TARGET, so the
# unscoped path was the only one that ever ran.
#
# METAREVIEW_TARGET still narrows to one target when the caller knows it. METAREVIEW_BASE picks
# the branch base; empty lets metareview resolve it the way every other command does.
TARGET="${METAREVIEW_TARGET:-}"
BASE="${METAREVIEW_BASE:-}"

# The host's payload arrives on stdin. The only field read is `stop_hook_active`, which the host
# sets when the session is ALREADY continuing because of a Stop hook — its documented purpose is
# loop prevention.
#
# Ignoring it is what produced the measured failure: on 2026-08-30 this hook fired NINE times in
# ten turns against a blocker the session could not clear by itself, and the run ended with
# is_error false and an empty result. A gate that cannot be satisfied and cannot be exited is not
# enforcement, it is a hang, and a hang teaches operators to remove the hook.
#
# So the gate yields on the second pass — and says so, on stderr, naming what it yielded on. A
# gate that stands down SILENTLY is the failure this whole layer exists to remove; one that stands
# down loudly leaves the evidence in the transcript, where the next reviewer and post-merge
# learning can both find it. Clearing the blockers, or recording a process override, remain the
# ways to finish without one.
# The read is BOUNDED. `cat` here blocks forever when stdin is an open pipe that never sends
# anything — which is any host that does not write a payload, and any manual invocation. A hook
# that hangs is worse than one that fails: the host waits, the session stops, and nothing says
# why. One second is far longer than a host needs to write a small JSON object, and the gate
# behaves exactly as before when nothing arrives.
PAYLOAD=""
if [ ! -t 0 ]; then
  IFS= read -r -d "" -t 1 PAYLOAD || true
fi
# Three fields, one per line: the loop flag, the session id, and the session's cwd. A value that
# is not a string, or that contains a newline, is dropped rather than allowed to shift the lines.
FIELDS="$(printf '%s' "$PAYLOAD" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    d = d if isinstance(d, dict) else {}
except Exception:
    d = {}
def s(k):
    v = d.get(k)
    return v if isinstance(v, str) and "\n" not in v and "\r" not in v else ""
print("yes" if d.get("stop_hook_active") else "")
print(s("session_id"))
print(s("cwd"))
' 2>/dev/null || true)"
{ IFS= read -r LOOPING; IFS= read -r SESSION_ID; IFS= read -r HOST_CWD; } <<EOF || true
$FIELDS
EOF

# Where the session is. The payload's cwd is the host's own statement of it, so it wins over the
# directory the host happened to start this process in. (A relative METAREVIEW_BIN is relative
# to that original directory, so it is remembered first.)
ORIG_PWD="$(pwd)"
if [ -n "$HOST_CWD" ] && [ -d "$HOST_CWD" ]; then
  cd "$HOST_CWD" 2>/dev/null || true
fi

# Host-independent loop guard (mr-j30). The host's `stop_hook_active` is the documented loop breaker,
# but Codex reports it FALSE during hook-driven continuations (captured 2026-09-26: dozens of block
# reasons, only four yields), so a host that never flips it would refuse this gate FOREVER — a hang,
# and a hang teaches operators to remove the hook. Remember, per session, how many times IN A ROW the
# gate has refused, and stand down once that reaches a limit, exactly as the host-driven second pass
# does. Two counts, because "the same blocker" is not the only way to loop: the SAME blocker set trips
# the small per-set limit, and ANY blocker set that keeps coming back trips a larger session-wide cap
# (a session that perturbs its blockers every turn would otherwise never repeat one). A pass clears the
# record, and a stand-down clears it too — so a later session is judged on its own blockers, never a spent
# count (a host that sends NO session id is the one exception: its key is the directory it reports, so
# sessions there share a count — still bounded, just shared). State lives
# in the user's PRIVATE cache dir (below), not the shared temp dir. The whole mechanism is best-effort: a failure to read or write it never changes the
# gate's decision beyond skipping the guard, so it can only ever fail toward blocking.
STOP_GUARD_LIMIT="${METAREVIEW_STOP_GUARD_LIMIT:-3}"
case "$STOP_GUARD_LIMIT" in ''|*[!0-9]*) STOP_GUARD_LIMIT=3 ;; esac
[ "$STOP_GUARD_LIMIT" -ge 1 ] 2>/dev/null || STOP_GUARD_LIMIT=3
STOP_GUARD_TOTAL="${METAREVIEW_STOP_GUARD_TOTAL:-6}"
case "$STOP_GUARD_TOTAL" in ''|*[!0-9]*) STOP_GUARD_TOTAL=6 ;; esac
[ "$STOP_GUARD_TOTAL" -ge 1 ] 2>/dev/null || STOP_GUARD_TOTAL=6
# Per-user PRIVATE location, not the shared temp dir: on a multi-user host /tmp is writable by others, who
# could plant or rewrite a counter file and force an early stand-down (a fail-open). If that cache dir is
# not writable (a read-only home), fall back to the repository's OWN git dir — writable wherever the session
# can commit — and only then to the temp dir, so an unwritable cache never leaves the guard inert.
if [ -n "${XDG_CACHE_HOME:-}" ]; then
  STOP_STATE_BASE="$XDG_CACHE_HOME"
elif [ -n "${HOME:-}" ]; then
  STOP_STATE_BASE="$HOME/.cache"
else
  STOP_STATE_BASE="${TMPDIR:-/tmp}"
fi
STOP_STATE_DIR="$STOP_STATE_BASE/metareview/stop-gate"
GIT_COMMON_DIR="$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"
if [ -z "$GIT_COMMON_DIR" ]; then
  GIT_COMMON_DIR="$(git rev-parse --git-common-dir 2>/dev/null || true)"
  [ -n "$GIT_COMMON_DIR" ] && [ "${GIT_COMMON_DIR#/}" = "$GIT_COMMON_DIR" ] && GIT_COMMON_DIR="$PWD/$GIT_COMMON_DIR"
fi
# The per-session key: the host's session id when it gives one. Otherwise the directory the host reported —
# the one stable handle a no-session host gives us — so the counter persists across a session's turns and the
# guard still bounds a loop there. Two sessions in one checkout then share a counter (a rare, no-session host
# is the only case); the alternative, keying on a process id, changes every turn and leaves the guard inert
# exactly where mr-j30 needs it. python3 hashes it; cksum is a dependency-free fallback.
STOP_KEY=""
if [ -n "$SESSION_ID" ] || [ -n "$HOST_CWD" ] || [ -n "$ORIG_PWD" ]; then
  STOP_KEY_SEED="${SESSION_ID:-${HOST_CWD:-$ORIG_PWD}}"
  STOP_KEY="$(printf '%s' "$STOP_KEY_SEED" | python3 -c 'import hashlib,sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest()[:32])' 2>/dev/null || true)"
  [ -z "$STOP_KEY" ] && STOP_KEY="$(printf '%s' "$STOP_KEY_SEED" | cksum 2>/dev/null | tr -d ' \t' || true)"
fi

# stop_guard_writable reports whether dir is a directory the current user OWNS and that no group or other
# can write to. umask 077 sets that on creation; this rejects a PRE-EXISTING directory (the shared temp-dir
# fallback is the attack) that another local user could have created to plant or rewrite a counter.
stop_guard_writable() {
  if ! { [ -d "$1" ] && [ -O "$1" ] && [ -w "$1" ]; }; then return 1; fi
  local mode
  mode="$(stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1" 2>/dev/null || true)"
  case "$mode" in ''|*[!0-7]*) return 1 ;; esac
  case "${mode: -1}" in 2|3|6|7) return 1 ;; esac  # other-writable
  case "${mode: -2:1}" in 2|3|6|7) return 1 ;; esac # group-writable
  return 0
}

# stop_guard_file prints this session's state file, creating its directory in the first WRITABLE location
# the current user owns (private cache dir, else the repository's git dir, else the temp dir). It is created
# lazily — a session that never blocks leaves nothing behind — and returns 1 when no location qualifies, so
# the caller skips the guard (which fails toward blocking).
stop_guard_file() {
  local dir
  [ -n "$STOP_KEY" ] || return 1
  for dir in "$STOP_STATE_DIR" "${GIT_COMMON_DIR:+$GIT_COMMON_DIR/metareview/stop-gate}" "${TMPDIR:-/tmp}/metareview/stop-gate"; do
    [ -n "$dir" ] || continue
    (umask 077; mkdir -p "$dir") 2>/dev/null || continue
    stop_guard_writable "$dir" || continue
    printf '%s/%s' "$dir" "$STOP_KEY"
    return 0
  done
  return 1
}

# block_is_repeat SIG returns 0 when this refusal repeats one too many times for this session —
# STOP_GUARD_LIMIT times in a row on the SAME SIG, or STOP_GUARD_TOTAL refusals of ANY kind in a row —
# and the caller then stands down. Otherwise it records the running counts and returns 1. An empty SIG
# (the blocker set could not be read, e.g. no python3) is its own signature '-': consecutive unreadable
# refusals are treated as the SAME set, so the per-set limit bounds them like any other. The mechanism
# only ever fails toward blocking (a SIG it cannot compute still counts).
block_is_repeat() {
  local sig="${1:--}" file prev_n=0 prev_sig="" prev_total=0 n total
  file="$(stop_guard_file)" || return 1
  if [ -f "$file" ]; then read -r prev_n prev_sig prev_total < "$file" 2>/dev/null || true; fi
  case "$prev_n" in ''|*[!0-9]*) prev_n=0 ;; esac
  case "$prev_total" in ''|*[!0-9]*) prev_total=0 ;; esac
  if [ "$prev_sig" = "$sig" ]; then n=$((prev_n + 1)); else n=1; fi
  total=$((prev_total + 1))
  printf '%s %s %s' "$n" "$sig" "$total" > "$file" 2>/dev/null || true
  [ "$n" -ge "$STOP_GUARD_LIMIT" ] || [ "$total" -ge "$STOP_GUARD_TOTAL" ]
}

# stop_guard_clear removes this session's record: on a pass (the gate cleared) and on a stand-down (the
# count is spent), so a later session or a later clean run is judged on its own blockers, never this one's.
# It never CREATES the state directory, so a session that never blocks leaves nothing behind.
stop_guard_clear() {
  local dir file
  [ -n "$STOP_KEY" ] || return 0
  for dir in "$STOP_STATE_DIR" "${GIT_COMMON_DIR:+$GIT_COMMON_DIR/metareview/stop-gate}" "${TMPDIR:-/tmp}/metareview/stop-gate"; do
    [ -n "$dir" ] || continue
    file="$dir/$STOP_KEY"
    [ -f "$file" ] && rm -f "$file" 2>/dev/null
  done
  return 0
}

# Opt-in (#194). The plugin registers this hook in EVERY session on the machine — unrelated projects,
# directories that are not repositories, the FSM judge's throwaway `codex exec` sessions. It gates only
# a repository that asked for it: `metareview setup --install-hooks` records metareview.stopGate=true in
# the repository's own config. Anywhere else it exits at once, silently, before even looking for the
# binary: a "metareview is not installed" block in a project that never opted in is a gate nobody
# asked for (and in a judge session it replaced the verdict's reasoning, #193).
if [ "$(git config --local --get metareview.stopGate 2>/dev/null || true)" != "true" ]; then
  # Standing down silently is right for a repository that never asked — but not for one that installed
  # metareview's git gate (core.hooksPath -> its user-level metareview/git-hooks/<id>, or a checkout's
  # .metareview/git-hooks before #173) and has lost the opt-in: an install from
  # before #194, or `git config --unset` from inside the session. That stands down LOUDLY, on stderr, so the
  # transcript shows the Stop gate was not applied and how to restore it.
  case "$(git config --get core.hooksPath 2>/dev/null || true)" in
    */metareview/git-hooks/*|*.metareview/git-hooks|*.metareview/git-hooks/)
      printf 'metareview: the Stop gate is NOT applied here — metareview'"'"'s git gate is installed but this repository has not opted into the Stop gate (metareview.stopGate is unset). Run `metareview setup --enable-stop-gate` to restore it.\n' >&2
      ;;
  esac
  exit 0
fi

# Find the binary the way the pre-push hook does. Plain `metareview` on PATH was the only lookup,
# so a project that builds its own into ./bin, with nothing on PATH, got "not installed". PATH
# still wins when it has one, exactly as in pre-push.
BIN="${METAREVIEW_BIN:-}"
if [ -z "$BIN" ]; then
  TOP="$(git rev-parse --show-toplevel 2>/dev/null || true)"
  if command -v metareview >/dev/null 2>&1; then
    # The path PATH resolved, not the bare name: a relative PATH entry (`bin`) would otherwise
    # name a different binary, or none, once the hook moves into a bound worktree.
    BIN="$(command -v metareview)"
    case "$BIN" in
      /*) ;;
      *) BIN="$PWD/$BIN" ;;
    esac
  elif [ -n "${CLAUDE_PROJECT_DIR:-}" ] && [ -x "$CLAUDE_PROJECT_DIR/bin/metareview" ]; then
    BIN="$CLAUDE_PROJECT_DIR/bin/metareview"
  elif [ -n "$TOP" ] && [ -x "$TOP/bin/metareview" ]; then
    BIN="$TOP/bin/metareview"
  else
    BIN="metareview"
  fi
fi

# The hook changes directory again below (into a bound worktree), so a relative path would stop
# naming the binary it was checked as: absolute from here on.
case "$BIN" in
  /*) ;;
  */*) BIN="$ORIG_PWD/$BIN" ;;
esac

if ! command -v "$BIN" >/dev/null 2>&1; then
  # Absent tooling is reported, never treated as a pass: a check that did not run must not read
  # as a check that found nothing wrong.
  if [ -n "$LOOPING" ] || block_is_repeat "metareview-not-installed"; then
    # The repeat pass. A missing binary is the ONE blocker a session can never clear from inside,
    # so refusing forever here is precisely the hang this yield exists to prevent.
    stop_guard_clear
    printf 'metareview: yielding after a repeated block — metareview is not installed, so the gate could not run.\n' >&2
    exit 0
  fi
  printf '{"decision":"block","reason":"metareview is not installed, so the review gate could not run. Install it or unset the hook deliberately."}\n'
  exit 0
fi

# The session may be bound to another worktree of this repository: the host reports the checkout
# it LAUNCHED in, and a session opened on main that works in a sibling worktree was evaluated
# against main on every turn — blocked on files it never touched, with no way to clear them.
# `metareview session resolve` answers from the binding (or with this checkout when there is
# none). An older CLI without `session` fails here and the hook simply stays where it is; its
# usage error is not the operator's business. A warning from a CLI that DID answer is.
LAUNCHED="$(git rev-parse --show-toplevel 2>/dev/null || pwd -P)"
LAUNCHED="$(cd "$LAUNCHED" 2>/dev/null && pwd -P || printf '%s' "$LAUNCHED")"
BOUND=""
CHECKED=""
# Asked even without a session id: resolve prints the root status will evaluate (which honours
# nested metareview markers the git toplevel does not), so the reason names what was checked.
# "bound" on its second line is the ONLY evidence of a binding — never inferred from paths.
RES_ERR="$(mktemp "${TMPDIR:-/tmp}/metareview-resolve.XXXXXX" 2>/dev/null || true)"
if RESOLVED="$("$BIN" session resolve ${SESSION_ID:+"$SESSION_ID"} 2>"${RES_ERR:-/dev/null}")"; then
  [ -n "$RES_ERR" ] && [ -s "$RES_ERR" ] && cat "$RES_ERR" >&2
  { IFS= read -r RES_DIR; IFS= read -r RES_MARK; } <<EOF || true
$RESOLVED
EOF
  if [ -n "$RES_DIR" ] && [ -d "$RES_DIR" ] && cd "$RES_DIR" 2>/dev/null; then
    CHECKED="$(pwd -P)"
    [ "${RES_MARK:-}" = "bound" ] && BOUND="yes"
  fi
fi
[ -n "$RES_ERR" ] && rm -f "$RES_ERR"
if [ -z "$CHECKED" ]; then
  CHECKED="$(git rev-parse --show-toplevel 2>/dev/null || pwd -P)"
  CHECKED="$(cd "$CHECKED" 2>/dev/null && pwd -P || printf '%s' "$CHECKED")"
fi
BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
export MRV_CHECKED="$CHECKED" MRV_BRANCH="$BRANCH" MRV_SESSION_ID="$SESSION_ID" MRV_BOUND="$BOUND"

# stdout ONLY. Capturing 2>&1 merged any diagnostic the CLI writes into the JSON, so json.load
# failed and the reason degraded silently to the static fallback — losing the blocker names the
# message exists to supply, exactly when something had gone wrong enough to warrant a diagnostic.
# stderr is kept, on stderr, where an operator still sees it.
if [ -n "$TARGET" ]; then
  OUT="$("$BIN" status --json --target "$TARGET")"
elif [ -n "$BASE" ]; then
  OUT="$("$BIN" status --json --scope branch --base "$BASE")"
else
  OUT="$("$BIN" status --json --scope branch)"
fi
CODE=$?

# A stable signature of the blocker set this refusal is about: the same blockers on the next turn are
# the livelock the guard exists to break, a different set means the session made progress. Bounded and
# fail-safe: any parse trouble yields an empty signature, which the guard treats as the empty set (its
# own signature, so it still counts toward both limits) — never a silent pass.
BLOCK_SIG=""
if [ "$CODE" -eq 1 ]; then
  BLOCK_SIG="$(printf '%s' "$OUT" | python3 -c '
import hashlib, json, sys
try:
    items = json.load(sys.stdin).get("must_clear") or []
except Exception:
    sys.exit(1)
rows = sorted((str(i.get("target", "")), str(i.get("verdict", ""))) for i in items if isinstance(i, dict))
sys.stdout.write(hashlib.sha256("\n".join("\t".join(r) for r in rows).encode()).hexdigest())
' 2>/dev/null || true)"
fi

# Exit 1 means "something must be cleared". Any other nonzero code means the gate itself failed, and a
# broken gate is NOT yielded on a host flag alone — a status that crashed on the second pass must not
# silently bypass enforcement — but the same host-independent guard bounds it (below), so a PERSISTENT
# failure stands down loudly instead of hanging the session forever.
if [ "$CODE" -eq 1 ] && { [ -n "$LOOPING" ] || block_is_repeat "$BLOCK_SIG"; }; then
  # Stand down: clear the record so a later session (or a later clean run) starts fresh.
  stop_guard_clear
  # Second pass: the host is already continuing because of this hook. Yield, loudly.
  printf 'metareview: yielding after a repeated block — the gate was not satisfied in %s.\n' "$CHECKED" >&2
  printf '%s' "$OUT" | python3 -c '
import json, sys
try:
    items = json.load(sys.stdin).get("must_clear") or []
except Exception:
    items = []
if items:
    print("metareview: %d blocker(s) remain unresolved:" % len(items), file=sys.stderr)
    for i in items[:10]:
        print("  - %s [%s]" % (i.get("target", "?"), i.get("verdict", "?")), file=sys.stderr)
    if len(items) > 10:
        print("  … and %d more" % (len(items) - 10), file=sys.stderr)
print("metareview: clear them, or record one with `metareview override request`.", file=sys.stderr)
' || true
  exit 0
fi

if [ "$CODE" -eq 0 ]; then
  # Nothing to clear: the host proceeds. Clear the loop-guard record too, so a session that later
  # blocks afresh is judged on that blocker set, not this one's stale count.
  stop_guard_clear
  # A pass in a BOUND worktree is still announced — a binding
  # moves the gate off the launch checkout, and one chosen to dodge its blockers must be visible.
  if [ -n "$BOUND" ]; then
    printf 'metareview: passed in bound worktree %s%s for session %s; the launch checkout %s was not evaluated.\n' \
      "$CHECKED" "${BRANCH:+ (branch $BRANCH)}" "$SESSION_ID" "$LAUNCHED" >&2
  fi
  exit 0
fi

# Exit 1 means "something must be cleared". Anything else is a failure of the check itself, and
# both block — but they are reported differently, because "you have work to do" and "the gate is
# broken" call for different responses from a human.
if [ "$CODE" -eq 1 ]; then
  # Name what must actually be cleared. The first version scraped `"target"` with sed and took
  # head -1, which finds reviews[0] — the FIRST review in the whole log, not a must_clear entry —
  # so the message routinely named a target that was not blocking anything and sent whoever read
  # it to the wrong place. must_clear is the field the decision is made on, so it is the field
  # the reason has to quote.
  # The WHOLE response is built by python and emitted with json.dumps, not assembled by printf.
  # A target reaches this text verbatim, and targets are review targets — task ids and file paths.
  # A path containing a double quote, a backslash or a newline (all legal on the platforms this
  # runs on) produced invalid JSON, and the host cannot act on a block decision it cannot parse:
  # the gate fails open at exactly the moment it is trying to close.
  RESPONSE="$(printf '%s' "$OUT" | python3 -c '
import json, os, re, sys

def reason(r):
    items = r.get("must_clear") or []
    if not items:
        return ""
    unreviewed = [i for i in items if i.get("verdict") == "UNREVIEWED"]
    if unreviewed:
        return " — %s has never been reviewed; run the appropriate metareview gate on it" % unreviewed[0].get("target", "the target")
    first = items[0]
    extra = "" if len(items) == 1 else " (and %d more)" % (len(items) - 1)
    return " on %s [%s]%s" % (first.get("target", "?"), first.get("verdict", "?"), extra)

def where():
    # WHICH checkout was evaluated. A session launched on one checkout and working in another
    # read these blockers as its own; naming the checkout makes the mismatch visible, and the
    # session id (from the host payload) lets the reason quote the exact command that fixes it.
    checked = os.environ.get("MRV_CHECKED", "")
    branch = os.environ.get("MRV_BRANCH", "")
    sid = os.environ.get("MRV_SESSION_ID", "")
    if not checked:
        return ""
    text = " Evaluated %s%s." % (checked, " (branch %s)" % branch if branch else "")
    if os.environ.get("MRV_BOUND"):
        text += " Session %s is bound to this worktree; `metareview session unbind %s` undoes it." % (sid, sid)
    elif re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,127}", sid):
        # Only an id `session bind` would accept is quoted into a command the agent may run.
        text += (" If the work of this session is in another worktree of this repository, run"
                 " `metareview session bind %s <worktree-path>` so this check evaluates that worktree"
                 " (its own pending reviews still apply)." % sid)
    return text

try:
    summary = reason(json.load(sys.stdin))
except Exception:
    summary = ""
print(json.dumps({
    "decision": "block",
    "reason": "metareview has unresolved blockers%s. Run `metareview status --json` to see what must be cleared, fix them, or record a process override with a reason.%s" % (summary, where()),
}))
' 2>/dev/null)"
  # If python is missing or died, still block — with a valid, static response. Failing to describe
  # the blockers must never become failing to block.
  if [ -z "$RESPONSE" ]; then
    RESPONSE='{"decision":"block","reason":"metareview has unresolved blockers. Run `metareview status --json` to see what must be cleared, fix them, or record a process override with a reason."}'
  fi
  printf '%s\n' "$RESPONSE"
else
  # A broken gate blocks — status failed to answer, and "did not run" is never "found nothing". But it
  # must not block FOREVER: a persistent failure (a corrupt ledger, a half-installed binary) is exactly the
  # unactionable-from-inside refusal the loop guard exists to bound, so the same best-effort guard applies,
  # keyed on the exit code. A transient failure recovers and a pass clears the record; only a persistent
  # one stands down, loudly.
  if block_is_repeat "broken-gate-$CODE"; then
    stop_guard_clear
    printf 'metareview: yielding after a repeated block — metareview status kept failing (exit %s), so the gate could not answer.\n' "$CODE" >&2
    exit 0
  fi
  printf '{"decision":"block","reason":"metareview status failed (exit %s), so the review gate could not answer. This is a broken gate, not a clean tree."}\n' "$CODE"
fi
exit 0
