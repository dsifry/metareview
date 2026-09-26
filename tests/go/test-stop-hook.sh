#!/usr/bin/env bash
# The Stop hook: the shim that makes the Completion Rule a gate rather than a sentence in
# CLAUDE.md. It had no test at all, which is how it shipped defaulting to an UNSCOPED status
# query — a livelock nobody could clear — while `--scope branch` already existed beside it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
HOOK="$ROOT/hooks/pre-finish.sh"

(cd "$ROOT" && go build -o "$TMP/mrv" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo/docs/metareview/reviews"
cd "$repo"
git init -q -b main
git config user.email t@e
git config user.name t
printf 'package p\n' > base.go
git add base.go
git -c commit.gpgsign=false commit -qm base
git checkout -q -b work
printf 'package p // changed\n' > changed.go
git add changed.go
git -c commit.gpgsign=false commit -qm work
head="$(git rev-parse HEAD)"

# A hook must always emit ONE line of valid JSON when it blocks, or the host cannot act on it.
assert_json_block() {  # assert_json_block <output> <substring>
  printf '%s' "$1" | python3 -c '
import json, sys
d = json.load(sys.stdin)
assert d["decision"] == "block", d
assert d["reason"], d
' || { echo "FAIL: hook did not emit a valid block decision: $1"; exit 1; }
  printf '%s' "$1" | grep -q "$2" || { printf "FAIL: reason missing %s: %s\n" "" "" >&2; exit 1; }
}

# 1. Absent tooling blocks. A check that did not run must never read as a check that found
#    nothing wrong.
out="$(METAREVIEW_BIN=definitely-not-installed bash "$HOOK")"
assert_json_block "$out" "not installed"

# 2. The branch changed a file no review has read, so the hook blocks — and says so in the words
#    that tell an operator what to do, distinctly from "you have findings to fix".
out="$(METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "never been reviewed"
printf '%s' "$out" | grep -q "changed.go" || { echo "FAIL: the block must name the unreviewed file: $out"; exit 1; }

# 3. Once a passing review records that it read the file, the hook lets the session finish. A
#    gate that cannot be satisfied is one an operator disables, which is worse than none.
printf '# metareview: pr-ready review\n\nRun ID: `mrv-1`\n\nTarget: `current branch`\n\nHead: `%s`\n\nCovered paths: `["base.go","changed.go"]`\n\n## Verdict\n\nPASS\n' \
  "$head" > docs/metareview/reviews/mrv-1-pr-ready.md
out="$(METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
if [ -n "$out" ]; then echo "FAIL: a reviewed branch must pass silently, got: $out"; exit 1; fi

# 4. A blocking review of this branch's own commits blocks, and is reported as a verdict rather
#    than as "never reviewed" — the two call for different things from whoever reads it.
printf '# metareview: task-done review\n\nRun ID: `mrv-2`\n\nTarget: `t-2`\n\nHead: `%s`\n\nCovered paths: `["changed.go"]`\n\n## Verdict\n\nNEEDS_REVISION\n' \
  "$head" > docs/metareview/reviews/mrv-2-task-done.md
out="$(METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "NEEDS_REVISION"

# 5. METAREVIEW_TARGET still narrows to one target when the caller knows it.
out="$(METAREVIEW_BIN="$TMP/mrv" METAREVIEW_TARGET=t-2 bash "$HOOK")"
assert_json_block "$out" "t-2"

# 6. The hook runs from wherever the session happens to be standing. Resolving against the
#    process cwd found no review logs at all and exited 0 — the gate bypassed by the entirely
#    ordinary act of working in a subdirectory, and bypassed silently.
mkdir -p "$repo/internal/deep"
out="$(cd "$repo/internal/deep" && METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "NEEDS_REVISION"

# 7. A gate that errors is broken, not clean, and says which of the two it is.
cat > "$TMP/broken" <<'EOF'
#!/bin/sh
echo "boom" >&2
exit 7
EOF
chmod +x "$TMP/broken"
out="$(METAREVIEW_BIN="$TMP/broken" bash "$HOOK")"
assert_json_block "$out" "could not answer"

# 8. The response must be valid JSON even when a blocker's target is hostile.
#    Targets are review targets — task ids and file paths — and a path may legally contain a
#    double quote, a backslash or a newline. The reason was assembled with printf around a raw
#    target, so such a path produced unparseable JSON and the host could not act on the block
#    decision: the gate failed OPEN at the moment it was trying to close.
cat > "$TMP/nasty" <<'EOF'
#!/bin/sh
printf '%s\n' '{"blocked":true,"must_clear":[{"target":"src/say \"hi\"\\back\nnext.go","verdict":"UNREVIEWED"}]}'
exit 1
EOF
chmod +x "$TMP/nasty"
out="$(METAREVIEW_BIN="$TMP/nasty" bash "$HOOK")"
printf '%s' "$out" | python3 -c '
import json, sys
d = json.load(sys.stdin)
assert d["decision"] == "block", d
assert "never been reviewed" in d["reason"], d
' || { echo "FAIL: a hostile target produced unparseable JSON: $out" >&2; exit 1; }

# 9. A blocking status whose body the summariser cannot read must still produce a valid block.
#    Failing to DESCRIBE the blockers must never become failing to block, so the response is a
#    static valid one rather than a broken string or an empty stdout the host reads as "allow".
cat > "$TMP/garbage" <<'EOF'
#!/bin/sh
echo "not json at all {{{"
exit 1
EOF
chmod +x "$TMP/garbage"
out="$(METAREVIEW_BIN="$TMP/garbage" bash "$HOOK")"
printf '%s' "$out" | python3 -c '
import json, sys
d = json.load(sys.stdin)
assert d["decision"] == "block", d
' || { echo "FAIL: unreadable status body did not produce a valid block: $out" >&2; exit 1; }

# 10-12. The loop-prevention contract. These use their OWN stub rather than the fixture repo,
#        because the assertions are about the hook's behaviour, not about whatever the previous
#        test left in the tree — a fixture inherited from four tests ago is how a test ends up
#        asserting something it does not exercise.
cat > "$TMP/blocking" <<'EOF'
#!/bin/sh
printf '%s\n' '{"blocked":true,"must_clear":[{"target":"internal/thing.go","verdict":"UNREVIEWED"}]}'
exit 1
EOF
chmod +x "$TMP/blocking"

# 10. The host says it is ALREADY continuing because of this hook. The gate yields — a gate that
#     cannot be satisfied and cannot be exited is a hang, and this one was measured firing nine
#     times in ten turns against a blocker the session could not clear by itself. It yields
#     LOUDLY: standing down silently would be the failure this whole layer exists to remove.
out="$(printf '{"stop_hook_active":true}' | METAREVIEW_BIN="$TMP/blocking" bash "$HOOK" 2>"$TMP/err")"
if [ -n "$out" ]; then
  echo "FAIL: yielding must not emit a block decision: $out" >&2; exit 1
fi
grep -q "yielding after a repeated block" "$TMP/err" || {
  echo "FAIL: the yield must be announced, not silent:"; cat "$TMP/err" >&2; exit 1; }
grep -q "internal/thing.go" "$TMP/err" || {
  echo "FAIL: the yield must name what was left unresolved:"; cat "$TMP/err" >&2; exit 1; }
grep -q "override request" "$TMP/err" || {
  echo "FAIL: the yield must name the recorded way out:"; cat "$TMP/err" >&2; exit 1; }

# 11. The FIRST pass still blocks, and so does an absent, false, or unparseable payload. Yielding
#     is for a repeat; a gate that stands down on every session is bypassed by ignoring it once.
for payload in '{"stop_hook_active":false}' '{}' 'not json at all' ''; do
  out="$(printf '%s' "$payload" | METAREVIEW_BIN="$TMP/blocking" bash "$HOOK")"
  assert_json_block "$out" "internal/thing.go"
done

# 12. A clean tree with stop_hook_active set is simply clean, and says nothing at all.
cat > "$TMP/clean" <<'EOF'
#!/bin/sh
printf '%s\n' '{"blocked":false,"must_clear":[]}'
exit 0
EOF
chmod +x "$TMP/clean"
out="$(printf '{"stop_hook_active":true}' | METAREVIEW_BIN="$TMP/clean" bash "$HOOK" 2>"$TMP/err2")"
if [ -n "$out" ] || [ -s "$TMP/err2" ]; then
  echo "FAIL: a clean tree must pass quietly: out=$out err=$(cat "$TMP/err2")" >&2; exit 1
fi

# 13. A BROKEN gate is never yielded past. Exit 1 means "something must be cleared"; any other
#     nonzero code means the check itself failed. Yielding on every nonzero code meant a status
#     that crashed on the second pass silently bypassed completion enforcement — the gate turning
#     itself off precisely when it had stopped working.
cat > "$TMP/crashing" <<'EOF'
#!/bin/sh
echo "internal error" >&2
exit 2
EOF
chmod +x "$TMP/crashing"
out="$(printf '{"stop_hook_active":true}' | METAREVIEW_BIN="$TMP/crashing" bash "$HOOK" 2>/dev/null)"
assert_json_block "$out" "could not answer"

# 14. A missing binary IS yielded past on the repeat, because it is the one blocker a session can
#     never clear from inside: refusing forever there is the hang this yield exists to prevent.
out="$(printf '{"stop_hook_active":true}' | METAREVIEW_BIN=definitely-not-installed bash "$HOOK" 2>"$TMP/err3")"
if [ -n "$out" ]; then
  echo "FAIL: an unclearable missing binary must yield on the repeat, not block: $out" >&2; exit 1
fi
grep -q "not installed" "$TMP/err3" || {
  echo "FAIL: the yield must say why it stood down:"; cat "$TMP/err3" >&2; exit 1; }
# ...and still blocks on the FIRST pass.
out="$(printf '{}' | METAREVIEW_BIN=definitely-not-installed bash "$HOOK")"
assert_json_block "$out" "not installed"

# 15. A diagnostic on stderr must not corrupt the JSON the reason is built from. Capturing 2>&1
#     merged them, json.load failed, and the message degraded to the static fallback — losing the
#     blocker names exactly when something had gone wrong enough to warrant a diagnostic.
cat > "$TMP/noisy" <<'EOF'
#!/bin/sh
echo "warning: something chatty" >&2
printf '%s\n' '{"blocked":true,"must_clear":[{"target":"internal/noisy.go","verdict":"UNREVIEWED"}]}'
exit 1
EOF
chmod +x "$TMP/noisy"
out="$(printf '{}' | METAREVIEW_BIN="$TMP/noisy" bash "$HOOK" 2>/dev/null)"
assert_json_block "$out" "internal/noisy.go"

# 16-20. The session is launched on one checkout and works in a sibling worktree. Captured from a
#        real Codex Stop event on 2026-09-26: the hook's $PWD and the payload's cwd BOTH named the
#        launch checkout (main), while the work was in another worktree — so the hook evaluated
#        main on every turn and blocked on files the session never touched. Nothing in the payload
#        names the worktree; the session binds it once, by the id the payload does carry.
keeper="$TMP/keeper"
git -C "$repo" worktree add -q -b keeper "$keeper" main
printf 'package p // keeper\n' > "$keeper/keeper.go"
git -C "$keeper" add keeper.go
git -C "$keeper" -c commit.gpgsign=false commit -qm keeper
keeper_head="$(git -C "$keeper" rev-parse HEAD)"
keeper_real="$(cd "$keeper" && pwd -P)"
repo_real="$(cd "$repo" && pwd -P)"
payload="{\"session_id\":\"sess-1\",\"cwd\":\"$repo\",\"stop_hook_active\":false}"

# 16. Unbound: the hook evaluates the launch checkout, says WHICH checkout it evaluated, and quotes
#     the exact command — session id included — that points it at the right worktree.
out="$(cd "$repo" && printf '%s' "$payload" | METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "NEEDS_REVISION"
printf '%s' "$out" | grep -qF "$repo_real" || { echo "FAIL: the block must name the checkout it evaluated: $out"; exit 1; }
printf '%s' "$out" | grep -qF "metareview session bind sess-1 " || { echo "FAIL: the block must quote the bind command: $out"; exit 1; }

# 16b. A session id `session bind` would refuse is never quoted into a command for the agent to
#      run — the block still names the checkout, but offers no bind line.
out="$(cd "$repo" && printf '{"session_id":"x; rm -rf ~","cwd":"%s"}' "$repo" | METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "NEEDS_REVISION"
if printf '%s' "$out" | grep -q "session bind"; then echo "FAIL: an unsafe session id was quoted into a command: $out"; exit 1; fi

# 16c. A subdirectory carrying its own metareview marker is evaluated as its own root (as before),
#      and the hook says so — without claiming a binding that does not exist.
mkdir -p "$repo/pkg/docs/metareview"
out="$(printf '{"session_id":"sess-9","cwd":"%s"}' "$repo/pkg" | METAREVIEW_BIN="$TMP/mrv" bash "$HOOK" 2>"$TMP/err16c" || true)"
if printf '%s' "$out" | grep -q "is bound" || grep -q "bound worktree" "$TMP/err16c"; then
  echo "FAIL: an unbound session was reported as bound: out=$out err=$(cat "$TMP/err16c")"; exit 1
fi
if [ -n "$out" ]; then
  printf '%s' "$out" | grep -qF "Evaluated $repo_real/pkg" || { echo "FAIL: the reason must name the root status evaluated: $out"; exit 1; }
fi
rm -rf "${repo:?}/pkg"

# 17. Bound: evaluated in the worktree, from the SAME launch checkout. The worktree's own pending
#     review surfaces — a binding selects a checkout, it never exempts one — and main's blocker
#     does not.
(cd "$repo" && "$TMP/mrv" session bind sess-1 "$keeper" >/dev/null)
out="$(cd "$repo" && printf '%s' "$payload" | METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "keeper.go"
printf '%s' "$out" | grep -qF "$keeper_real" || { echo "FAIL: the block must name the bound worktree: $out"; exit 1; }
if printf '%s' "$out" | grep -q "NEEDS_REVISION\|session bind"; then
  echo "FAIL: a bound session must not report the launch checkout's blockers or re-offer the bind: $out"; exit 1
fi

# 18. Once the worktree's work is reviewed, the bound session finishes — silently — even though
#     the launch checkout is still blocked. This is the livelock the yield used to paper over.
mkdir -p "$keeper/docs/metareview/reviews"
printf '# metareview: pr-ready review\n\nRun ID: `mrv-k`\n\nTarget: `current branch`\n\nHead: `%s`\n\nCovered paths: `["keeper.go"]`\n\n## Verdict\n\nPASS\n' \
  "$keeper_head" > "$keeper/docs/metareview/reviews/mrv-k-pr-ready.md"
#     It passes WITHOUT a block decision, but never silently: a bound pass says on stderr which
#     worktree passed and that the launch checkout was not evaluated, so a binding chosen to dodge
#     the launch checkout's blockers is visible in the transcript.
out="$(cd "$repo" && printf '%s' "$payload" | METAREVIEW_BIN="$TMP/mrv" bash "$HOOK" 2>"$TMP/err4")"
if [ -n "$out" ]; then
  echo "FAIL: a bound, reviewed worktree must not block: out=$out"; exit 1
fi
if ! { grep -qF "$keeper_real" "$TMP/err4" && grep -qF "$repo_real" "$TMP/err4" && grep -q "not evaluated" "$TMP/err4"; }; then
  echo "FAIL: a bound pass must name the worktree and the unevaluated launch checkout:"; cat "$TMP/err4"; exit 1
fi

# 18b. A relative METAREVIEW_BIN still works once the hook has moved into the bound worktree.
mkdir -p "$repo/tools" && cp "$TMP/mrv" "$repo/tools/mrv"
printf 'tools/\n' >> "$(git -C "$repo" rev-parse --git-common-dir)/info/exclude"
out="$(cd "$repo" && printf '%s' "$payload" | METAREVIEW_BIN=tools/mrv bash "$HOOK" 2>"$TMP/err4b")"
if [ -n "$out" ]; then echo "FAIL: a relative METAREVIEW_BIN broke after the bind cd: $out"; exit 1; fi
rm -rf "${repo:?}/tools"

# 18c. Likewise a metareview found through a RELATIVE PATH entry: the bound worktree has no such
#      directory, so the name must be pinned to the binary that was found, before any cd.
mkdir -p "$repo/relbin" && cp "$TMP/mrv" "$repo/relbin/metareview"
printf 'relbin/\n' >> "$(git -C "$repo" rev-parse --git-common-dir)/info/exclude"
relpath="relbin:$(dirname "$(command -v git)"):$(dirname "$(command -v python3)"):/usr/bin:/bin"
out="$(cd "$repo" && printf '%s' "$payload" | env -u METAREVIEW_BIN PATH="$relpath" bash "$HOOK" 2>"$TMP/err4c")"
if [ -n "$out" ]; then echo "FAIL: a relative PATH entry broke after the bind cd: $out"; exit 1; fi
rm -rf "${repo:?}/relbin"

# 19. The payload's cwd is preferred over the process directory: a host may start the hook
#     somewhere else entirely and still report where the session is.
out="$(cd "$TMP" && printf '{"cwd":"%s"}' "$repo" | METAREVIEW_BIN="$TMP/mrv" bash "$HOOK")"
assert_json_block "$out" "NEEDS_REVISION"

# 20. A binding whose worktree is gone falls back to the launch checkout — and says so.
git -C "$repo" worktree remove --force "$keeper"
out="$(cd "$repo" && printf '%s' "$payload" | METAREVIEW_BIN="$TMP/mrv" bash "$HOOK" 2>"$TMP/err5")"
assert_json_block "$out" "NEEDS_REVISION"
grep -q "cannot be used" "$TMP/err5" || { echo "FAIL: a stale binding must be reported:"; cat "$TMP/err5"; exit 1; }
(cd "$repo" && "$TMP/mrv" session unbind sess-1 >/dev/null)

# 21. An older CLI without `session` still gates: resolve fails, the hook stays where it is, and
#     the old binary's complaint does not leak into the transcript.
cat > "$TMP/old" <<EOF
#!/bin/sh
if [ "\$1" = session ]; then echo "Unknown command: session" >&2; exit 2; fi
exec "$TMP/mrv" "\$@"
EOF
chmod +x "$TMP/old"
out="$(cd "$repo" && printf '%s' "$payload" | METAREVIEW_BIN="$TMP/old" bash "$HOOK" 2>"$TMP/err6")"
assert_json_block "$out" "NEEDS_REVISION"
if grep -q "Unknown command" "$TMP/err6"; then echo "FAIL: an old CLI's usage error leaked:"; cat "$TMP/err6"; exit 1; fi

# 22. With no METAREVIEW_BIN and nothing on PATH, the hook finds the checkout's own bin/metareview
#     — as the pre-push hook does — instead of blocking on "not installed".
minpath="$(dirname "$(command -v git)"):$(dirname "$(command -v python3)"):/usr/bin:/bin"
if PATH="$minpath" command -v metareview >/dev/null 2>&1; then
  echo "test-stop-hook: skipping 22 (a metareview is on the minimal PATH)"
else
  # Ignored, as a project's own build is: an untracked binary would itself be an unreviewed file.
  mkdir -p "$repo/bin" && cp "$TMP/mrv" "$repo/bin/metareview"
  printf 'bin/\n' >> "$(git -C "$repo" rev-parse --git-common-dir)/info/exclude"
  out="$(cd "$repo/internal/deep" && printf '{}' | env -u METAREVIEW_BIN -u CLAUDE_PROJECT_DIR PATH="$minpath" bash "$HOOK")"
  assert_json_block "$out" "NEEDS_REVISION"
  rm -rf "${repo:?}/bin"
fi

echo "test-stop-hook: ok"
