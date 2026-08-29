# metareview status

Show repository review mode and integration status:

```bash
metareview status
```

Use status before deciding which generated artifacts to commit. Review artifacts under `docs/metareview/` and git-visible learning state should be committed; transient `.metareview/findings.jsonl`, `.metareview/runs.jsonl` and `.metareview/shards/` stay local. Committed shard review results live in `docs/metareview/shards/`, and FSM export bundles in `docs/metareview/fsm/`; `.metareview/runs/` (FSM runs) stays local and ignores itself.

Arguments: `$ARGUMENTS`
