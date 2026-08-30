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

# --target scopes the answer to the work in hand. Without it `blocked` spans the entire review
# history, so the hook would refuse an agent because of work it never touched — a livelock, not a
# gate, and the reason this hook could not ship before.
TARGET="${METAREVIEW_TARGET:-}"

BIN="${METAREVIEW_BIN:-metareview}"
if ! command -v "$BIN" >/dev/null 2>&1; then
  # Absent tooling is reported, never treated as a pass: a check that did not run must not read
  # as a check that found nothing wrong.
  printf '{"decision":"block","reason":"metareview is not installed, so the review gate could not run. Install it or unset the hook deliberately."}\n'
  exit 0
fi

if [ -n "$TARGET" ]; then
  OUT="$("$BIN" status --json --target "$TARGET" 2>&1)"
else
  OUT="$("$BIN" status --json 2>&1)"
fi
CODE=$?

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
  SUMMARY="$(printf '%s' "$OUT" | python3 -c '
import json, sys
try:
    r = json.load(sys.stdin)
except Exception:
    sys.exit(0)
items = r.get("must_clear") or []
if not items:
    sys.exit(0)
unreviewed = [i for i in items if i.get("verdict") == "UNREVIEWED"]
if unreviewed:
    print(" — %s has never been reviewed; run the appropriate metareview gate on it" % unreviewed[0].get("target", "the target"))
else:
    first = items[0]
    extra = "" if len(items) == 1 else " (and %d more)" % (len(items) - 1)
    print(" on %s [%s]%s" % (first.get("target", "?"), first.get("verdict", "?"), extra))
' 2>/dev/null)"
  printf '{"decision":"block","reason":"metareview has unresolved blockers%s. Run `metareview status --json` to see what must be cleared, fix them, or record a process override with a reason."}\n' "${SUMMARY:-}"
else
  printf '{"decision":"block","reason":"metareview status failed (exit %s), so the review gate could not answer. This is a broken gate, not a clean tree."}\n' "$CODE"
fi
exit 0
