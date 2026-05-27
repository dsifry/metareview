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

Report:

- repository mode
- git presence
- Beads presence
- metaswarm presence
- service inventory presence
- whether `.metareview` state exists

Also report whether the current generated artifacts should be committed or kept local:

- commit `docs/metareview/reviews/`, `docs/metareview/context/`, `docs/metareview/learning/`, `.metareview/knowledge/metareview.jsonl`, `.metareview/calibration.jsonl`, and `.metareview/learning-runs.jsonl`
- keep `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, and other transient `.metareview/` state local
