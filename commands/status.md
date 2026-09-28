# metareview status

Show repository review mode and integration status:

```bash
metareview status
```

`--json` emits the machine-readable form instead: `{version, mode, git, beads, metaswarm, reviews,
must_clear, blocked}`, where `must_clear` names every review with unresolved blockers (target, run
id, verdict, kind, log path, blocking count, attempt of max) and `blocked` is the single boolean a
host hook branches on. It exits 1 when something must be cleared and 0 when nothing does, so a hook
can gate on the exit code alone.

```bash
metareview status --json
```

Abandoned FSM runs (left mid-loop) are scoped to the branch in hand (#177): a run blocks the branch `fsm init`
recorded for it and any branch stacked on it, and survives rebase, amend and rename (a rename is read from the
branch's reflog, so not where none is kept, as in a bare repository by default). Plain `status` names each blocking
run's branch, which for a stacked branch may be its base. Runs of other live branches
(`otherBranchRuns`) and of deleted ones (`orphanedRuns`) never block and are only counted. `--all`, with or without
`--json`, also lists them (`elsewhere`), grouped by branch with the run directory to delete — and never changes the
exit code.

```bash
metareview status --all
metareview status --json --all
```

Use status before deciding which generated artifacts to commit. Review artifacts under `docs/metareview/` and git-visible learning state should be committed; transient `.metareview/findings.jsonl`, `.metareview/runs.jsonl` and `.metareview/shards/` stay local. Committed shard review results live in `docs/metareview/shards/`, and FSM export bundles in `docs/metareview/fsm/`; FSM runs live in git's common directory (`<git-common-dir>/metareview/runs/` — the main checkout's `.git/metareview/runs/`) and are never tracked.

Arguments: `$ARGUMENTS`
