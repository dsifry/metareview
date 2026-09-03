#!/usr/bin/env bash
# Claude Code SessionStart hook — READ-ONLY. It NEVER mutates git config: the user installs the git-native
# review gate deliberately (`metareview setup --install-hooks`, which is interactive and non-destructive).
# This hook only CHECKS whether the gate is installed (core.hooksPath -> this clone's hooks/git) and, when it
# is not, reminds the agent that an unreviewed push will not be blocked until it is installed. That is the
# safety net for a fresh clone: git hooks do not auto-install (git's security model), so without this a clone
# could be silently ungated.
set -uo pipefail

ROOT="${CLAUDE_PROJECT_DIR:-}"
[ -n "$ROOT" ] || ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
[ -n "$ROOT" ] || exit 0 # not a git repo / unknown root — nothing to check

# The installer materializes the hooks into .metareview/git-hooks and points core.hooksPath there.
WANT="$ROOT/.metareview/git-hooks"
# The effective value (local > global > system), matching what the installer treats as "in effect".
CUR="$(git -C "$ROOT" config --get core.hooksPath 2>/dev/null || true)"
case "$CUR" in
  /*) CURABS="$CUR" ;;
  "") CURABS="" ;;
  *)  CURABS="$ROOT/$CUR" ;; # git resolves a relative hooksPath against the worktree root
esac
# Normalize both before comparing, so equivalent spellings — `$ROOT/./hooks/git`, `hooks/git/`, a `..`
# segment — collapse to the same path and do not read as "not installed". Lexical only (the paths need not
# exist); if python3 is unavailable the fallback just strips a trailing slash, preserving prior behavior.
norm() { python3 -c 'import os,sys; print(os.path.normpath(sys.argv[1]))' "$1" 2>/dev/null || printf '%s' "${1%/}"; }
# Installed means BOTH: core.hooksPath points at the target AND the materialized scripts are actually there.
# .metareview/git-hooks is git-ignored, so the scripts can vanish while config still points at them — that is
# an inert gate, and the reminder must fire so the agent reinstalls, not stay silent on a config match alone.
if [ -n "$CURABS" ] && [ "$(norm "$CURABS")" = "$(norm "$WANT")" ] && [ -x "$WANT/pre-push" ] && [ -x "$WANT/post-commit" ]; then
  exit 0 # installed and the hooks are present — nothing to say
fi

MSG="metareview: the git-native review gate is NOT installed (or its hook scripts are missing) on this repo — an unreviewed 'git push' will NOT be blocked. To install it (non-destructive; refuses on conflict): run \`metareview setup --install-hooks\` interactively, or \`metareview setup --install-hooks --yes\` headlessly, or \`--dry-run\` to preview."
CTX="$(printf '%s' "$MSG" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))' 2>/dev/null)"
[ -n "$CTX" ] || CTX='"metareview: run `metareview setup --install-hooks` to enable the review gate (an unreviewed push is not blocked until you do)."'
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":%s}}\n' "$CTX"
exit 0
