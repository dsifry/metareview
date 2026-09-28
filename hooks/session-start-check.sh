#!/usr/bin/env bash
# Claude Code SessionStart hook — READ-ONLY. It NEVER mutates git config: the user installs the git-native
# review gate deliberately (`metareview setup --install-hooks`, which is interactive and non-destructive).
# This hook only CHECKS whether the gate is installed (core.hooksPath -> metareview's gate scripts) and, when it
# is not, reminds the agent that an unreviewed push will not be blocked until it is installed. That is the
# safety net for a fresh clone: git hooks do not auto-install (git's security model), so without this a clone
# could be silently ungated.
set -uo pipefail

ROOT="${CLAUDE_PROJECT_DIR:-}"
[ -n "$ROOT" ] || ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
[ -n "$ROOT" ] || exit 0 # not a git repo / unknown root — nothing to check

# The installer materializes the hooks under the user's data home, named by their content
# (${XDG_DATA_HOME:-~/.local/share}/metareview/git-hooks/<id>, #173), and points core.hooksPath there. This script
# cannot compute that id, so it asks the question that matters: are core.hooksPath's scripts metareview's gate?
# The effective value (local > global > system), matching what the installer treats as "in effect".
CUR="$(git -C "$ROOT" config --get core.hooksPath 2>/dev/null || true)"
case "$CUR" in
  /*) CURABS="$CUR" ;;
  "") CURABS="" ;;
  *)  CURABS="$ROOT/$CUR" ;; # git resolves a relative hooksPath against the worktree root
esac
# Normalize before comparing, so equivalent spellings — `$ROOT/./hooks/git`, `hooks/git/`, a `..` segment —
# collapse to the same path. Lexical only (the paths need not exist); if python3 is unavailable the fallback just
# strips a trailing slash.
norm() { python3 -c 'import os,sys; print(os.path.normpath(sys.argv[1]))' "$1" 2>/dev/null || printf '%s' "${1%/}"; }
# metareview's gate lives in $1: both scripts executable, and pre-push is the gate (its marker).
gate_in() { [ -x "$1/pre-push" ] && [ -x "$1/post-commit" ] && grep -q "review gate --push" "$1/pre-push" 2>/dev/null; }
# $1 is a location metareview materializes into, now or before #173: the user-level content-addressed dirs, a
# checkout's .metareview/git-hooks, or this clone's committed hooks/git.
metareview_path() {
  case "$(norm "$1")" in
    */metareview/git-hooks/*|*/.metareview/git-hooks|"$(norm "$ROOT/hooks/git")") return 0 ;;
  esac
  return 1
}
# Installed means core.hooksPath points at metareview's gate scripts AND they are actually there: they can vanish
# while config still points at them — an inert gate, and the reminder must fire so the agent reinstalls.
if [ -n "$CURABS" ] && gate_in "$CURABS"; then
  MSG=""
  case "$(norm "$CURABS")" in
    */.metareview/git-hooks|"$(norm "$ROOT/hooks/git")")
      # A pre-#173 location inside a checkout: moving that checkout would silently ungate every linked worktree.
      MSG="metareview: the git-native push gate is installed at a checkout-local location ($CUR), which stops working if that checkout moves. Run \`metareview setup --install-hooks\` to migrate it to the user-level location." ;;
  esac
  # The push gate is installed. The Stop gate is a separate, per-repository opt-in (#194): an install from
  # before it has no opt-in, and would lose the Stop gate on upgrade with nothing in the session saying so.
  if [ "$(git -C "$ROOT" config --local --get metareview.stopGate 2>/dev/null || true)" != "true" ]; then
    MSG="${MSG:+$MSG }metareview: the git-native push gate is installed, but this repository has not opted into the Stop gate, so session completion is not gated here. Run \`metareview setup --enable-stop-gate\` to opt in."
  fi
  [ -n "$MSG" ] || exit 0 # both gates in place — nothing to say
else
  MSG="metareview: the git-native review gate is NOT installed (or its hook scripts are missing) on this repo — an unreviewed 'git push' will NOT be blocked. To install it (non-destructive; refuses on conflict): run \`metareview setup --install-hooks\` interactively, or \`metareview setup --install-hooks --yes\` headlessly, or \`--dry-run\` to preview."
  # Another tool owns core.hooksPath (husky, lefthook, beads): --install-hooks refuses there, and --force would
  # override that tool. The Stop gate does not need core.hooksPath, so name the opt-in that works (#194).
  # "Another tool" means what the installer treats as a conflict — NOT metareview's own locations, which it
  # reclaims without --force (internal/setup isOurHookPath) when they are gone or still hold its gate.
  FOREIGN=""
  if [ -n "$CURABS" ]; then
    FOREIGN="yes"
    if metareview_path "$CURABS" && { [ ! -d "$CURABS" ] || grep -q "review gate --push" "$CURABS/pre-push" 2>/dev/null; }; then
      FOREIGN=""
    fi
  fi
  if [ -n "$FOREIGN" ] && [ "$(git -C "$ROOT" config --local --get metareview.stopGate 2>/dev/null || true)" != "true" ]; then
    MSG="$MSG core.hooksPath is set to $CUR by another tool, so install will refuse; to gate session completion here without changing it, run \`metareview setup --enable-stop-gate\`."
  fi
fi
CTX="$(printf '%s' "$MSG" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))' 2>/dev/null)"
[ -n "$CTX" ] || CTX='"metareview: run `metareview setup --install-hooks` to enable the review gate (an unreviewed push is not blocked until you do)."'
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":%s}}\n' "$CTX"
exit 0
