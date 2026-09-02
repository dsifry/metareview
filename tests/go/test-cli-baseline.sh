#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

go test ./...

version="$(go run ./cmd/metareview --version)"
test "$version" = "$(node -p "require('./package.json').version")"

help="$(go run ./cmd/metareview --help)"
printf '%s\n' "$help" | grep -q 'metareview setup --check'
printf '%s\n' "$help" | grep -q 'metareview context build <path>'
printf '%s\n' "$help" | grep -q 'metareview context diff'
printf '%s\n' "$help" | grep -q 'metareview evidence run --'
printf '%s\n' "$help" | grep -q 'metareview evidence import --github-checks'
printf '%s\n' "$help" | grep -q 'metareview review artifact <path>'
printf '%s\n' "$help" | grep -q -- '--scaffold-only'
printf '%s\n' "$help" | grep -q 'metareview review task-done'
printf '%s\n' "$help" | grep -q 'metareview review epic-ready'
printf '%s\n' "$help" | grep -q 'metareview review pr-ready'

# status --json is the contract a host hook branches on: valid JSON on stdout, a `blocked`
# flag, and exit 1 when something must be cleared so the common decision needs no parsing.
# Exercised here rather than in a Go test because the exit decision lives in main's dispatch.
clean="$(mktemp -d)"
trap 'rm -rf "$clean"' EXIT
go build -o "$clean/mrv" ./cmd/metareview

out="$(cd "$clean" && ./mrv status --json)" || { echo "FAIL: status --json on a clean tree exited nonzero"; exit 1; }
printf '%s' "$out" | grep -q '"blocked": false' || { echo "FAIL: clean tree must not be blocked"; exit 1; }
printf '%s' "$out" | grep -q '"must_clear": \[\]' || { echo "FAIL: clean tree must have an empty must_clear"; exit 1; }

mkdir -p "$clean/docs/metareview/reviews"
printf '# metareview: task-done review\n\nRun ID: `mrv-x`\nTarget: `t-1`\n\n## Verdict\n\nNEEDS_REVISION\n' \
  > "$clean/docs/metareview/reviews/mrv-x-task-done-t-1.md"
if (cd "$clean" && ./mrv status --json >/dev/null); then
  echo "FAIL: status --json must exit nonzero when something must be cleared"; exit 1
fi
# the binary exits 1 by design here, and pipefail would fail the pipeline before grep runs
blocked_out="$( (cd "$clean" && ./mrv status --json 2>/dev/null) || true )"
printf '%s' "$blocked_out" | grep -q 't-1' || { echo "FAIL: the blocker must be named in the output"; exit 1; }
printf '%s' "$blocked_out" | grep -q '"blocked": true' || { echo "FAIL: blocked must be true"; exit 1; }

# --target scopes that answer to the work in hand. Without it `blocked` spans the whole review
# history, so a Stop hook wired to it refuses an agent because of work it never touched: a
# livelock rather than a gate, and the reason the hooks could not ship before now.
printf '# metareview: task-done review\n\nRun ID: `mrv-y`\nTarget: `t-2`\n\n## Verdict\n\nNEEDS_REVISION\n' \
  > "$clean/docs/metareview/reviews/mrv-y-task-done-t-2.md"

scoped="$( (cd "$clean" && ./mrv status --json --target t-1 2>/dev/null) || true )"
printf '%s' "$scoped" | grep -q 't-1' || { echo "FAIL: the scoped report must name its own blocker"; exit 1; }
printf '%s' "$scoped" | grep -q 't-2' && { echo "FAIL: scoping must not leak another target's blocker"; exit 1; }
printf '%s' "$scoped" | grep -q '"target": "t-1"' || { echo "FAIL: the report must say what it was scoped to"; exit 1; }

# A target NO REVIEW COVERS is blocked, as UNREVIEWED. This assertion used to be the opposite,
# and that was the hole: the narrower the scope an agent claimed, the more certainly the gate let
# it through, because an unknown target matched no log and produced an empty must_clear. Naming a
# file nobody had reviewed was the reliable way to be told everything was fine.
unreviewed="$( (cd "$clean" && ./mrv status --json --target t-untouched 2>/dev/null) || true )"
printf '%s' "$unreviewed" | grep -q 'UNREVIEWED' || {
  echo "FAIL: a target no review covers must be reported UNREVIEWED, not cleared"; exit 1; }
if (cd "$clean" && ./mrv status --json --target t-untouched >/dev/null 2>&1); then
  echo "FAIL: a target no review covers must not exit 0"; exit 1
fi

# ...and a target that WAS reviewed and passed still lets work through, or the gate is a livelock.
printf '# metareview: task-done review\n\nRun ID: `mrv-z`\nTarget: `t-3`\n\n## Verdict\n\nPASS\n' \
  > "$clean/docs/metareview/reviews/mrv-z-task-done-t-3.md"
if ! (cd "$clean" && ./mrv status --json --target t-3 >/dev/null 2>&1); then
  echo "FAIL: a reviewed, passing target must exit 0"; exit 1
fi

# A real branch, so --scope branch has something to resolve. gitcontext takes the base from
# merge-base with main, so the work has to be on a branch OFF main, not on main itself.
(
  cd "$clean"
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
)

# --scope branch answers about the work in hand: this branch's own commits, and the files it
# changed that no passing review has read. It is the scope a Stop hook needs — unscoped, the
# answer spans the whole review history and refuses every session for work it never touched.
if (cd "$clean" && ./mrv status --json --scope branch >/dev/null 2>&1); then
  echo "FAIL: a branch whose changed files nobody reviewed must block"; exit 1
fi
branch_out="$( (cd "$clean" && ./mrv status --json --scope branch 2>/dev/null) || true )"
printf '%s' "$branch_out" | grep -q 'UNREVIEWED' || {
  echo "FAIL: changed files with no covering review must be reported UNREVIEWED"; exit 1; }
printf '%s' "$branch_out" | grep -q '"target": "branch ' || {
  echo "FAIL: the report must say it was scoped to a branch"; exit 1; }
# t-1's blocker is not on this branch's commits, so it must not appear in the branch answer.
printf '%s' "$branch_out" | grep -q '"run_id": "mrv-x"' && {
  echo "FAIL: an unrelated blocker leaked into the branch-scoped answer"; exit 1; }
# The spelling with an equals sign works too. The `|| true` this used to carry discarded the
# result, so the form was invoked and never actually asserted.
eq_out="$( (cd "$clean" && ./mrv status --json --scope=branch 2>/dev/null) || true )"
printf '%s' "$eq_out" | grep -q '"target": "branch ' || {
  echo "FAIL: --scope=branch must scope the same way --scope branch does: $eq_out"; exit 1; }
# And an explicit base, which had no end-to-end coverage at all.
base_sha="$( (cd "$clean" && git rev-parse HEAD~1) 2>/dev/null || true )"
if [ -n "$base_sha" ]; then
  base_out="$( (cd "$clean" && ./mrv status --json --scope branch --base "$base_sha" 2>/dev/null) || true )"
  printf '%s' "$base_out" | grep -q "\"target\": \"branch $(printf '%s' "$base_sha" | cut -c1-8)" || {
    echo "FAIL: --base must set the base the report names: $base_out"; exit 1; }
fi
# A malformed scope is refused rather than silently treated as unscoped.
if (cd "$clean" && ./mrv status --json --scope 2>/dev/null); then
  echo "FAIL: --scope with no value must be refused"; exit 1
fi
if (cd "$clean" && ./mrv status --json --scope nonsense 2>/dev/null); then
  echo "FAIL: an unknown scope must be refused"; exit 1
fi

# The gate must also survive being run from a subdirectory, which is where a Stop hook actually
# runs: it inherits the session's cwd, and resolving there used to find no review logs at all.
mkdir -p "$clean/internal/deep"
if (cd "$clean/internal/deep" && "$clean/mrv" status --json >/dev/null 2>&1); then
  echo "FAIL: status from a subdirectory must still see the repository's blockers"; exit 1
fi
# ...while the unscoped answer over the same tree still blocks.
if (cd "$clean" && ./mrv status --json >/dev/null 2>&1); then
  echo "FAIL: the unscoped answer must still block"; exit 1
fi
# A malformed invocation is refused rather than silently treated as unscoped.
if (cd "$clean" && ./mrv status --json --target 2>/dev/null); then
  echo "FAIL: --target with no value must be refused"; exit 1
fi

# review prompt builds the adversarial-review prompt for a branch's changed files. Exercised here (a
# behavioral test) because the handler's flag parsing and label logic live in main's dispatch, so a plain
# `go test` never runs it — covering it keeps cmd/metareview above its coverage floor.
promptrepo="$(mktemp -d)"
trap 'rm -rf "$clean" "$promptrepo"' EXIT
(
  cd "$promptrepo"
  git init -q -b main
  printf 'package p\n' > a.go
  git add a.go
  git -c user.email=t@e -c user.name=t commit -qm base
  git checkout -q -b work
  printf 'package p\nvar X = 1\n' > a.go
  git add a.go
  git -c user.email=t@e -c user.name=t commit -qm change
)
(cd "$promptrepo" && "$clean/mrv" review prompt --base main) | grep -q 'Adversarial review' \
  || { echo "FAIL: review prompt --base must render the prompt"; exit 1; }
(cd "$promptrepo" && "$clean/mrv" review prompt) | grep -q 'Adversarial review' \
  || { echo "FAIL: review prompt without --base must render the prompt"; exit 1; }
if (cd "$promptrepo" && "$clean/mrv" review prompt --bogus 2>/dev/null); then
  echo "FAIL: review prompt with an unknown option must exit nonzero"; exit 1
fi

# review gate is the deterministic decision the git-native hooks stand on: the CLI flag parsing and the
# staged/all/push scope dispatch live in main, so a plain `go test` never runs them. Covering them here
# keeps cmd/metareview above its floor and pins the hooks' actual entry point.
gaterepo="$(mktemp -d)"
trap 'rm -rf "$clean" "$promptrepo" "$gaterepo"' EXIT
(
  cd "$gaterepo"
  git init -q -b main
  printf 'package p\n' > base.go
  git add base.go
  git -c user.email=t@e -c user.name=t commit -qm base
  git checkout -q -b work
  # A COMMITTED unreviewed change, so the push gate (which measures committed content, not the index) has
  # something to gate. A staged-but-uncommitted file is NOT part of a push and no longer blocks it.
  printf 'package p\nvar C = 1\n' > committed.go
  git add committed.go
  git -c user.email=t@e -c user.name=t commit -qm feature
)

# Nothing staged: the commit gate has nothing to gate → exit 0.
(cd "$gaterepo" && "$clean/mrv" review gate) \
  || { echo "FAIL: review gate with nothing staged must exit 0"; exit 1; }

# A staged, unreviewed file blocks the commit gate (exit 1) and is named on stderr.
(cd "$gaterepo" && printf 'package p\nvar S = 1\n' > staged.go && git add staged.go)
if (cd "$gaterepo" && "$clean/mrv" review gate >/dev/null 2>&1); then
  echo "FAIL: a staged unreviewed file must block the commit gate"; exit 1
fi
gate_err="$( (cd "$gaterepo" && "$clean/mrv" review gate 2>&1 >/dev/null) || true )"
printf '%s' "$gate_err" | grep -q 'staged.go' \
  || { echo "FAIL: the commit gate must name the unreviewed file: $gate_err"; exit 1; }

# --base threads through to the review-prompt command in the fix-it message.
gate_base_err="$( (cd "$gaterepo" && "$clean/mrv" review gate --base main 2>&1 >/dev/null) || true )"
printf '%s' "$gate_base_err" | grep -q -- '--base main' \
  || { echo "FAIL: --base must thread through the commit-gate message: $gate_base_err"; exit 1; }

# --push scopes to the whole branch; an unreviewed branch blocks the push.
if (cd "$gaterepo" && "$clean/mrv" review gate --push >/dev/null 2>&1); then
  echo "FAIL: an unreviewed branch must block a push"; exit 1
fi
push_err="$( (cd "$gaterepo" && "$clean/mrv" review gate --push 2>&1 >/dev/null) || true )"
printf '%s' "$push_err" | grep -q 'push blocked' \
  || { echo "FAIL: --push must report a whole-branch block: $push_err"; exit 1; }

# --all mirrors `git commit -a`: an UNSTAGED tracked modification is gated, which the staged scope misses.
(cd "$gaterepo" && git -c user.email=t@e -c user.name=t commit -qm staged) # clean index
(cd "$gaterepo" && printf 'package p\nvar S = 2\n' > staged.go)           # modified, unstaged
(cd "$gaterepo" && "$clean/mrv" review gate >/dev/null 2>&1) \
  || { echo "FAIL: staged scope must not block an unstaged change (a plain commit would not write it)"; exit 1; }
if (cd "$gaterepo" && "$clean/mrv" review gate --all >/dev/null 2>&1); then
  echo "FAIL: --all must gate an unstaged tracked modification"; exit 1
fi

# An unknown gate option is refused.
if (cd "$gaterepo" && "$clean/mrv" review gate --bogus >/dev/null 2>&1); then
  echo "FAIL: review gate with an unknown option must exit nonzero"; exit 1
fi

# setup --install-hooks: total user control, non-destructive, agent-safe. All exercised without a TTY (the
# way an agent or CI invokes it), so the interactive prompt branch is intentionally not reached here.
hookrepo="$(mktemp -d)"
trap 'rm -rf "$clean" "$promptrepo" "$gaterepo" "$hookrepo"' EXIT
(cd "$hookrepo" && git init -q -b main)

# --dry-run previews the plan and changes nothing.
dry="$( (cd "$hookrepo" && "$clean/mrv" setup --install-hooks --dry-run) )"
printf '%s' "$dry" | grep -q 'core.hooksPath' \
  || { echo "FAIL: --dry-run must print the plan: $dry"; exit 1; }
printf '%s' "$dry" | grep -q 'dry run' \
  || { echo "FAIL: --dry-run must say it changed nothing: $dry"; exit 1; }
test -z "$( (cd "$hookrepo" && git config --local --get core.hooksPath) || true )" \
  || { echo "FAIL: --dry-run must not set core.hooksPath"; exit 1; }

# No TTY and no --yes (an agent): print directions, change NOTHING, never hang.
noyes="$( (cd "$hookrepo" && "$clean/mrv" setup --install-hooks </dev/null) )"
printf '%s' "$noyes" | grep -q 'NOTHING was changed' \
  || { echo "FAIL: no-TTY, no --yes must change nothing and say so: $noyes"; exit 1; }
test -z "$( (cd "$hookrepo" && git config --local --get core.hooksPath) || true )" \
  || { echo "FAIL: the no-TTY path must not set core.hooksPath"; exit 1; }

# --yes installs headlessly and sets core.hooksPath.
inst="$( (cd "$hookrepo" && "$clean/mrv" setup --install-hooks --yes) )"
printf '%s' "$inst" | grep -q 'Installed' \
  || { echo "FAIL: --install-hooks --yes must install: $inst"; exit 1; }
test -n "$( (cd "$hookrepo" && git config --local --get core.hooksPath) )" \
  || { echo "FAIL: --yes must set core.hooksPath"; exit 1; }

# Idempotent: a second --yes reports already-installed, no error.
again="$( (cd "$hookrepo" && "$clean/mrv" setup --install-hooks --yes) )"
printf '%s' "$again" | grep -q 'Already installed' \
  || { echo "FAIL: a second install must be a no-op: $again"; exit 1; }

# A foreign core.hooksPath is a conflict: not overridden without --force, overridden with it.
(cd "$hookrepo" && git config --local core.hooksPath .githooks-foreign)
if (cd "$hookrepo" && "$clean/mrv" setup --install-hooks --yes >/dev/null 2>&1); then
  echo "FAIL: a foreign core.hooksPath must block install without --force"; exit 1
fi
forced="$( (cd "$hookrepo" && "$clean/mrv" setup --install-hooks --yes --force) )"
printf '%s' "$forced" | grep -q 'Installed' \
  || { echo "FAIL: --force must override a conflict: $forced"; exit 1; }

# Uninstall honors --dry-run (previews, changes nothing) — it must NOT unset core.hooksPath.
(cd "$hookrepo" && git config --local core.hooksPath "$hookrepo/hooks/git") # our own, so uninstall would act
un_dry="$( (cd "$hookrepo" && "$clean/mrv" setup --uninstall-hooks --dry-run) )"
printf '%s' "$un_dry" | grep -q 'dry run' \
  || { echo "FAIL: --uninstall-hooks --dry-run must preview: $un_dry"; exit 1; }
test -n "$( (cd "$hookrepo" && git config --local --get core.hooksPath) )" \
  || { echo "FAIL: --uninstall-hooks --dry-run must NOT unset core.hooksPath"; exit 1; }

# Uninstall with no TTY and no --yes must change nothing (a non-interactive caller can't silently disable it).
un_noyes="$( (cd "$hookrepo" && "$clean/mrv" setup --uninstall-hooks </dev/null) )"
printf '%s' "$un_noyes" | grep -q 'NOTHING was changed' \
  || { echo "FAIL: no-TTY, no --yes uninstall must change nothing: $un_noyes"; exit 1; }
test -n "$( (cd "$hookrepo" && git config --local --get core.hooksPath) )" \
  || { echo "FAIL: no-TTY uninstall must NOT unset core.hooksPath"; exit 1; }

# --install-hooks and --uninstall-hooks together is refused (exit 2).
if (cd "$hookrepo" && "$clean/mrv" setup --install-hooks --uninstall-hooks >/dev/null 2>&1); then
  echo "FAIL: --install-hooks --uninstall-hooks together must exit nonzero"; exit 1
fi

# --uninstall-hooks --yes actually unsets it.
un_yes="$( (cd "$hookrepo" && "$clean/mrv" setup --uninstall-hooks --yes) )"
printf '%s' "$un_yes" | grep -q 'uninstalled' \
  || { echo "FAIL: --uninstall-hooks --yes must uninstall: $un_yes"; exit 1; }
test -z "$( (cd "$hookrepo" && git config --local --get core.hooksPath) || true )" \
  || { echo "FAIL: --uninstall-hooks --yes must unset core.hooksPath"; exit 1; }

# Nothing to uninstall once it is unset: it reports so and needs no --yes (it changes nothing).
un_none="$( (cd "$hookrepo" && "$clean/mrv" setup --uninstall-hooks </dev/null) )"
printf '%s' "$un_none" | grep -qi 'nothing to uninstall' \
  || { echo "FAIL: uninstall on an unset core.hooksPath must report nothing to do: $un_none"; exit 1; }
