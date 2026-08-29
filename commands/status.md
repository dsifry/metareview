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

Use status before deciding which generated artifacts to commit. Review artifacts under `docs/metareview/` and git-visible learning state should be committed; transient `.metareview/findings.jsonl`, `.metareview/runs.jsonl` and `.metareview/shards/` stay local. Committed shard review results live in `docs/metareview/shards/`, and FSM export bundles in `docs/metareview/fsm/`; `.metareview/runs/` (FSM runs) stays local and ignores itself.

Arguments: `$ARGUMENTS`
