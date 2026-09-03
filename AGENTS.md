# metareview Agent Instructions

These instructions apply to coding agents working in this repository or in repositories that install metareview.

**Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) first** — the map of metareview's shapes and patterns
(the nested review types, the two review engines — deterministic gate vs FSM `review-lenses` — the
git-native gate, the state/evidence model, cross-agent integration, and the durable conventions and
gotchas). It exists so you don't reinvent wheels; keep it current when those shapes change.

## Required Review Gates

Run metareview before claiming completion:

- Specs, plans, designs, decompositions, and docs: `metareview review artifact <path>`.
- Local task or chunk complete: `metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>`.
- Epic or parent task ready to land: `metareview review epic-ready <epic-id-or-path>`.
- PR ready to push or merge: `metareview review pr-ready --base <base-ref>`.
- After PR merge: `metareview learn --post-merge <pr-number> --base <pre-merge-ref>`.

Use `go run ./cmd/metareview ...` when running from a source checkout without a built `bin/metareview`.

## Blocker Policy

Do not claim completion while any blocking finding remains open.

Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr. For `metareview fsm`: `3` = the FSM needs the host to do a node's work; `1` + `GATE_FAILED` = run `resume_hint` (it forks a child — a new run id); `1` + `ERR_*` = read `code` (`detail` is data); `2` = nothing was recorded, fix the input and retry unless it is a consent or escalation code, which waits for a human; `STOPPED`/`DONE` are terminal. FSM escalation is per fork lineage: forking an ancestor or re-running `init` on the same base is a human decision.

Lifecycle gate verdicts have this contract:

- `PASS`: proceed.
- `PASS_ADVISORY`: proceed only when the review explicitly reports zero blocking findings.
- `NEEDS_REVISION`: fix blockers, then re-run the same gate with `--previous-run <run-id>`.
- `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.

Advisory notes can be recorded for later, but blockers are current work.

When a blocker cannot be fixed and the workflow is deliberately stepped outside of, record the exception
instead of working around it:

- `metareview override request <finding-id> --reason "<text>" [--escalation "<text>"]` — available to
  whoever drives the run, including an orchestrating agent. It does **not** clear the gate: the finding
  keeps blocking and `metareview override list --pending` exits nonzero, so CI stays red.
- `metareview override grant <finding-id> --reason "<text>"` — the acknowledgement, and it must come from
  outside the workflow (a human, or an authority explicitly designated as one). A reviewing agent never
  grants an override on findings it produced, and the actor that requested one is refused if it also tries
  to grant it. `--by` is audit metadata, not authentication: it records who claims to have acted, and a
  local CLI cannot verify that. Enforce the boundary with whatever authenticates actors in your
  environment when it matters.

Both halves record actor, timestamp and reason and are rendered under "Process Overrides" in
`docs/metareview/FINDINGS.md`. An override is never a fix: `fixedInRunId` stays empty, so exceptions can be
analysed separately from resolutions.

## Evidence Policy

For task-done and pr-ready reviews, provide evidence:

```bash
tmp_evidence="$(mktemp)"
printf '%s\n' \
  'Verification evidence:' \
  '- go test ./... exited 0' \
  '- bash tests/run-all.sh exited 0' \
  '- git diff --check exited 0' > "$tmp_evidence"
metareview review task-done <task-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
```

Evidence must reflect commands actually run in the current workspace.

## Artifact Policy

Commit durable artifacts:

- `docs/metareview/reviews/`
- `docs/metareview/context/`
- `docs/metareview/learning/`
- `docs/metareview/shards/` (committed shard review results)
- `.beads/knowledge/metareview.jsonl` when Beads owns knowledge
- `.metareview/knowledge/metareview.jsonl` in standalone fallback mode
- `.metareview/calibration.jsonl`
- `.metareview/learning-runs.jsonl`
- `docs/metareview/fsm/` (FSM export bundles)

Keep transient state local:

- `.metareview/findings.jsonl`
- `.metareview/runs.jsonl`
- `.metareview/runs/` (FSM runs; self-ignoring, incl. `.torn/`)
- `.metareview/shards/` (transient prompt packs; self-ignoring)
- generated binaries such as `bin/metareview`

## Metaswarm Fit

When metaswarm is present, it remains the lifecycle owner. Follow metaswarm's decomposition, Superpowers, Beads, and PR shepherding process, and insert metareview as the deeper review harness at artifact, task-done, epic-ready, pr-ready, and post-merge checkpoints.

When metaswarm is absent, use `metareview setup --bootstrap-prereqs --dry-run` before proposing local prerequisites or registries such as `docs/SERVICE_INVENTORY.md`.

### Sharded review (diffs over the context limit)

An exclude-filtered diff over 120,000 bytes cannot be held in one review context. metareview measures the real
branch diff, cuts it into shards, and writes a prompt pack per shard under
`.metareview/shards/<scope>/<target-slug>/<planHash>/`, with a `plan.json` naming every shard, its
hash, and the directory the results belong in.

1. Run the gate once. It reports `NEEDS_REVISION` with the context-risk blocker and writes the packs.
2. Read `plan.json`. Review one subagent per `shard-<id>.md` against `rubrics/task-done-review-rubric.md`,
   and one more over `cross-shard.md` when there is more than one shard.
3. Write a result per shard to `docs/metareview/shards/<scope>/<target-slug>/shard-<id>.<shardHash>.result.json`
   and, for a multi-shard plan, `cross-shard.<planHash>.result.json`. The pack states the exact
   contract.
4. Re-run the gate with `--previous-run <run-id>`. With every shard covered and the aggregate
   passing, the context-risk blocker becomes advisory and the deterministic lints run over the whole
   branch diff.

Set `--max-attempts` on the **first** run of the chain; mid-chain it is ignored. Commit the results
with the review log. Editing a file invalidates only its own shard's result: re-review that shard
and the cross-shard pack, and leave the rest. Local (staged, worktree, untracked) content is in no
pack, so on task-done commit or remove it first — an untracked file over 4,000 bytes raises
`UNTRACKED_TRUNCATED`, which shard results can never satisfy.
