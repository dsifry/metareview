#!/usr/bin/env bash
# AC-2.1 (#174): in the bare-clone + worktrees layout (`git clone --bare repo.git && git -C repo.git worktree add ../main
# main`) the FSM and the gates work: fsm init, advance to a terminal state, record-lenses --from-run, pr-ready and
# status all succeed. Before #173/#174 fsm refused with ERR_NOT_A_REPO "the main worktree is bare".
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
(cd "$ROOT" && go build -o "$TMP/mr" ./cmd/metareview) # before HOME changes: go's caches live under it
MR="$TMP/mr"
export XDG_DATA_HOME="$TMP/xdg" HOME="$TMP/home" GIT_CONFIG_NOSYSTEM=1
export OPENAI_API_KEY=unused # init checks the judge is configured; an empty finding list never calls it
mkdir -p "$HOME"
git config --global user.email t@example.com
git config --global user.name t
git config --global commit.gpgsign false

# A bare repository with one commit, and a linked worktree as the only checkout.
git init -q --bare -b main "$TMP/repo.git"
git clone -q "$TMP/repo.git" "$TMP/seed" 2>/dev/null
(cd "$TMP/seed" && git commit -q --allow-empty -m base && git push -q origin main)
git -C "$TMP/repo.git" worktree add -q "$TMP/main" main
cd "$TMP/main"
[ "$(git rev-parse --is-bare-repository)" = false ] || { echo "FAIL: setup: the worktree must not be bare"; exit 1; }
printf 'x\n' > file.txt
git add file.txt && git commit -q -m change

field() { python3 -c 'import json,sys; d=json.load(sys.stdin); v=d
for k in sys.argv[1].split("."): v=v.get(k) if isinstance(v,dict) else None
print("" if v is None else v)' "$1"; }

out="$("$MR" fsm init --workflow review-loop --base HEAD~1 --var JUDGE=gpt-5.2 --var JUDGE_EFFORT=medium)" \
  || { echo "FAIL: fsm init in a bare layout: $out"; exit 1; }
ID="$(printf '%s' "$out" | field run_id)"
[ -n "$ID" ] || { echo "FAIL: no run id: $out"; exit 1; }
[ -d "$TMP/repo.git/metareview/runs/$ID" ] || { echo "FAIL: the run must live in the bare repository's common dir"; exit 1; }

"$MR" fsm advance --run "$ID" >/dev/null || [ $? -eq 3 ] # discover: NEEDS_INPUT
printf '{"findings":[]}' > "$TMP/empty.json"
"$MR" fsm record node-output --run "$ID" --node discover --data "$TMP/empty.json" >/dev/null
out="$("$MR" fsm advance --run "$ID")" || { echo "FAIL: advance to terminal: $out"; exit 1; }
[ "$(printf '%s' "$out" | field status)" = DONE ] || { echo "FAIL: the run must reach a terminal state: $out"; exit 1; }

"$MR" review record-lenses --scope pr-ready --base HEAD~1 --verdict PASS --mode subagent-adjudicated \
  --from-run "$ID" --lenses security >/dev/null || { echo "FAIL: record-lenses --from-run in a bare layout"; exit 1; }
"$MR" evidence run -- true > "$TMP/ev.out" 2>&1
tail -1 "$TMP/ev.out" > "$TMP/ev.json"
"$MR" review pr-ready --base HEAD~1 --evidence "$TMP/ev.json" > "$TMP/pr.out" 2>&1 \
  || { echo "FAIL: pr-ready in a bare layout:"; cat "$TMP/pr.out"; exit 1; }
"$MR" status --json > "$TMP/status.json" || { echo "FAIL: status in a bare layout:"; cat "$TMP/status.json"; exit 1; }
"$MR" status > /dev/null || { echo "FAIL: plain status in a bare layout"; exit 1; }

# A second worktree shares the store: it reads the run the first one made.
git -C "$TMP/repo.git" worktree add -q -b other "$TMP/other" main
(cd "$TMP/other" && "$MR" fsm state --run "$ID" >/dev/null) || { echo "FAIL: a sibling worktree must read the shared run"; exit 1; }

# A worktree that leaves a run mid-flight and is then removed: the run belongs to its branch (#177), so no remaining
# worktree is blocked over it, and `status --all` lists it under that branch.
git -C "$TMP/repo.git" worktree add -q -b gone "$TMP/gone" main
GONE="$(cd "$TMP/gone" && "$MR" fsm init --workflow review-loop --base HEAD~1 --var JUDGE=gpt-5.2 --var JUDGE_EFFORT=medium | field run_id)"
(cd "$TMP/gone" && "$MR" fsm advance --run "$GONE" >/dev/null) || [ $? -eq 3 ] # left at discover
git -C "$TMP/repo.git" worktree remove --force "$TMP/gone"
for wt in "$TMP/main" "$TMP/other"; do
  (cd "$wt" && "$MR" status --json > "$TMP/st.json") || { echo "FAIL: $wt must not be blocked by a gone worktree's run:"; cat "$TMP/st.json"; exit 1; }
  (cd "$wt" && "$MR" status --json --all > "$TMP/st.json") || { echo "FAIL: --all must not change the exit in $wt:"; cat "$TMP/st.json"; exit 1; }
  if ! grep -Eq "\"runId\": ?\"$GONE\"" "$TMP/st.json" || ! grep -Eq '"branch": ?"gone"' "$TMP/st.json"; then
    echo "FAIL: $wt: status --all must list $GONE under branch gone"; cat "$TMP/st.json"; exit 1
  fi
done

# From the bare directory itself there is no checkout at all: fsm still refuses, with the reason.
out="$(cd "$TMP/repo.git" && "$MR" fsm state 2>&1)" && { echo "FAIL: fsm in the bare dir itself must refuse: $out"; exit 1; }
printf '%s' "$out" | grep -q '"reason":"bare"' || { echo "FAIL: the refusal must name the bare reason: $out"; exit 1; }

echo "test-bare-layout: ok"
