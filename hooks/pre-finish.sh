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
LOOPING="$(printf '%s' "$PAYLOAD" | python3 -c '
import json, sys
try:
    print("yes" if json.load(sys.stdin).get("stop_hook_active") else "")
except Exception:
    print("")
' 2>/dev/null || true)"

BIN="${METAREVIEW_BIN:-metareview}"
if ! command -v "$BIN" >/dev/null 2>&1; then
  # Absent tooling is reported, never treated as a pass: a check that did not run must not read
  # as a check that found nothing wrong.
  if [ -n "$LOOPING" ]; then
    # The repeat pass. A missing binary is the ONE blocker a session can never clear from inside,
    # so refusing forever here is precisely the hang this yield exists to prevent.
    printf 'metareview: yielding after a repeated block — metareview is not installed, so the gate could not run.\n' >&2
    exit 0
  fi
  printf '{"decision":"block","reason":"metareview is not installed, so the review gate could not run. Install it or unset the hook deliberately."}\n'
  exit 0
fi

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

# Exit 1 means "something must be cleared". Any other nonzero code means the gate itself failed,
# and a broken gate must NEVER be yielded past: yielding on every nonzero code meant a status that
# crashed on the second pass silently bypassed completion enforcement altogether.
if [ "$CODE" -eq 1 ] && [ -n "$LOOPING" ]; then
  # Second pass: the host is already continuing because of this hook. Yield, loudly.
  printf 'metareview: yielding after a repeated block — the gate was not satisfied.\n' >&2
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
  exit 0   # nothing to clear: the host proceeds
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
import json, sys

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

try:
    summary = reason(json.load(sys.stdin))
except Exception:
    summary = ""
print(json.dumps({
    "decision": "block",
    "reason": "metareview has unresolved blockers%s. Run `metareview status --json` to see what must be cleared, fix them, or record a process override with a reason." % summary,
}))
' 2>/dev/null)"
  # If python is missing or died, still block — with a valid, static response. Failing to describe
  # the blockers must never become failing to block.
  if [ -z "$RESPONSE" ]; then
    RESPONSE='{"decision":"block","reason":"metareview has unresolved blockers. Run `metareview status --json` to see what must be cleared, fix them, or record a process override with a reason."}'
  fi
  printf '%s\n' "$RESPONSE"
else
  printf '{"decision":"block","reason":"metareview status failed (exit %s), so the review gate could not answer. This is a broken gate, not a clean tree."}\n' "$CODE"
fi
exit 0
