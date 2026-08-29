---
name: status
description: Show metareview repository mode, available integrations, and unresolved review state.
---

# metareview status

Use when the user asks for metareview status or installation health.

Run:

```bash
metareview status
```

For a host hook or any programmatic caller, use the machine-readable form instead:

```bash
metareview status --json
```

It emits `{version, mode, git, beads, metaswarm, reviews, must_clear, blocked}`. `must_clear` names
every review with unresolved blockers (target, run id, verdict, kind, log path, blocking count,
attempt of max) and `blocked` is the single boolean to branch on. Exit 1 means something must be
cleared, 0 means nothing does, so a hook can gate on the exit code alone.

Report:

- repository mode
- git presence
- Beads presence
- metaswarm presence
- service inventory presence
- whether `.metareview` state exists

Also report whether the current generated artifacts should be committed or kept local:

- commit `docs/metareview/reviews/`, `docs/metareview/context/`, `docs/metareview/learning/`, `docs/metareview/shards/`, `docs/metareview/fsm/` (FSM export bundles), `.metareview/knowledge/metareview.jsonl`, `.metareview/calibration.jsonl`, and `.metareview/learning-runs.jsonl`
- keep `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `.metareview/runs/` (FSM runs, self-ignoring), `.metareview/shards/` (transient prompt packs), and other transient `.metareview/` state local
