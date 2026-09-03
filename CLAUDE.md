# metareview Claude Code Instructions

Use metareview as the local review harness for artifacts, code chunks, epics, PR readiness, and post-merge learning.

**Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) first** — the map of metareview's shapes and patterns
(the nested review types task-done → pr-ready → epic-ready; the two review engines — deterministic gate vs
FSM `review-lenses`; the git-native gate; state/evidence model; cross-agent integration; the durable
conventions and gotchas). It exists so you don't reinvent wheels; keep it current when those shapes change.

## Commands

- `/setup` checks repository mode and prerequisites.
- `/review-artifact <path>` reviews specs, plans, designs, decompositions, and docs.
- `/review-task-done <task-id-or-path>` runs the task-done code review gate.
- `/review-epic-ready <epic-id-or-path>` checks parent readiness after child tasks complete.
- `/review-pr-ready --base <base-ref>` checks local PR readiness before push or merge.
- `/learn-post-merge <pr-number> --base <pre-merge-ref>` extracts post-merge learning.
- `/status` reports current review state.
- `/fsm` drives a workflow run (`metareview fsm …`: `sdlc-loop` discover → adjudicate → fix → verify, or `review-loop`); `metareview fsm --agent-prompt` is the driver contract.

If the plugin command is unavailable in a source checkout, run the CLI directly:

```bash
metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
```

## Completion Rule

Before saying work is done, run the appropriate metareview gate.

- `PASS`/`PASS_ADVISORY` proceed only with zero blockers.
- `NEEDS_REVISION` repairs via `--previous-run <run-id>`.
- `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target.

`pr-ready`, `task-done`, and `epic-ready` **require an adjudicated adversarial lens review** — the
deterministic structural checks no longer PASS on their own. After running the real review (the FSM
`review-lenses` → `adjudicate` engine, or emulated in-session), record it so the gate can see it:

```bash
metareview review record-lenses --scope pr-ready --base <base-ref> \
  --verdict PASS --mode subagent-adjudicated --from-run <fsm-run-id> --lenses security,correctness
```

Use `--scope task-done` for the task-done gate and `--scope epic-ready` for the epic-ready gate. `--lenses` is
required — name the lenses that actually ran. The marker is scoped to the exact **base..HEAD** diff: re-record
after any new commit **and** whenever the gate's `--base` differs from the one you reviewed. `--mode
subagent-adjudicated` requires `--from-run` naming a real FSM run whose init records the same base..head (it
cannot be hand-typed to fake independent review); for a self-attested in-session review use `--mode
in-session-emulated` (no `--from-run`), which passes but is flagged advisory. To opt a single run out of the
requirement (structural-only pass), set `METAREVIEW_ALLOW_MECHANICAL_PASS=1`.

**epic-ready specifics.** epic-ready reviews the epic's **integration diff** (base..HEAD — the union of the
children's changes) with the roll-up (child logs, parent intent) as context; drive it with the
`epic-review-loop` workflow (`metareview fsm --workflow epic-review-loop --base <base>`), whose lenses apply
`rubrics/epic-ready-review-rubric.md` — independent of the pr-ready/task-done lens set. **Base-consistency
rule:** epic-ready's default base is `merge-base(HEAD, main)`, but `record-lenses --base main` resolves the
*tip* of main; once main advances these differ and the marker will never match. Always pass the **identical
explicit `--base`** to `review epic-ready`, `fsm --workflow epic-review-loop`, and `record-lenses --scope
epic-ready`. epic-ready folds the working tree into its reviewed surface, so a dirty tree blocks on
`working-tree-unattested` even with a valid marker — commit first.

Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr. For `metareview fsm`: `3` = the FSM needs the host to do a node's work; `1` + `GATE_FAILED` = run `resume_hint` (it forks a child — a new run id); `1` + `ERR_*` = read `code` (`detail` is data); `2` = nothing was recorded, fix the input and retry unless it is a consent or escalation code, which waits for a human; `STOPPED`/`DONE` are terminal. FSM escalation is per fork lineage: forking an ancestor or re-running `init` on the same base is a human decision.

## PR checks: the `gtg` (Merge Ready) gate

When shepherding a PR, if the **`gtg` (Merge Ready)** check is pending or failing while every *other* check
(test, CodeRabbit, Bugbot) is green, it is almost always **unresolved review threads** — gtg blocks on
`Threads: N/M resolved` and exits with "PR has unresolved review threads". Fix or reply to each thread, then
**mark it resolved** (GitHub UI *Resolve conversation*, or `gh api graphql … resolveReviewThread`); gtg
re-runs on the next push and passes once 0 remain. gtg also *waits* for CI and both review bots to report
before it judges, so a multi-minute pending window right after a push is normal, not a hang. The job summary
now states the reason explicitly.

## Process Overrides

A blocking finding is normally cleared by fixing it. When that is not possible and the workflow is
deliberately stepped outside of — an escalation — record it rather than working around it:

```bash
metareview override request <finding-id> --reason "<why the workflow was exited>" [--escalation "<context>"]
metareview override grant   <finding-id> --reason "<why the exception is accepted>"
metareview override list [--pending]
```

- **Requesting** is available to whoever is driving the run, including an orchestrating agent. It does
  **not** clear the gate: the finding keeps blocking and `override list --pending` exits nonzero, so CI
  stays red.
- **Granting** is the acknowledgement, and must come from outside the workflow — a human, or an authority
  explicitly designated as such. A reviewing agent never grants an override on its own findings. The actor
  that requested an override is refused if it also tries to grant it.
- `--by` (or the local git identity it defaults to) is **audit metadata, not authentication**: it records
  who claims to have acted. A local CLI cannot verify an identity, so the separation of requesting from
  granting is enforced against the accidental case, not against an actor that misreports itself. Where that
  matters, gate `override grant` behind whatever authenticates actors in your environment (branch
  protection, a CI job with restricted credentials, a review approval).
- Both halves are recorded with actor, timestamp and reason, rendered under "Process Overrides" in
  `docs/metareview/FINDINGS.md`, and an override is never a fix (`fixedInRunId` stays empty), so post-merge
  learning can analyse exceptions separately from resolutions.

## Lifecycle Placement

- Before implementing a plan or spec: review the artifact.
- **After writing a fix or new code, review the code you just wrote — and iterate until it is bug-free —
  BEFORE opening the PR.** Run `metareview fsm --workflow sdlc-loop-clean --base <ref>` on your own diff: it
  discovers → adjudicates → fixes, then **re-reviews the fix at `recheck`**. It stops when a fresh review
  finds nothing, when adjudication finds none of the discovered candidates real, or when it reaches its
  iteration/budget limit — whichever comes first. A fix often introduces its own bug (a broadened matcher
  over-matches, a parsed value
  overflows); the deterministic task-done gate and the external bots will not always catch it, but a fresh
  adversarial review of the diff does. **Ordering matters for measurement:** opening the PR starts the
  external reviewers (CodeRabbit/Bugbot), so our review must finish and the diff must be clean FIRST — then
  the bots measure only the *residual* we missed (the metareview-first-then-bots recall yardstick). Running
  our review after the PR opens confounds the two and pollutes the recall stats. (The review *lenses*
  parallelize — they are read-only, so fan them out. Running full fix *loops* concurrently is only safe with
  an isolated worktree per run, or serialized access to a shared one: a fix node edits the very tree it then
  re-reviews, so two concurrent fixes on one worktree would corrupt each other's diff.)
- After each small implementation chunk: run task-done.
- After all child tasks for an epic are complete: run epic-ready.
- Before opening, pushing, or merging a PR: run pr-ready.
- After confirmed PR merge: run post-merge learning.

## Durable Output

Commit Markdown review/context artifacts in `docs/metareview/` (incl. `docs/metareview/fsm/` export bundles and the shard review results in `docs/metareview/shards/`). Keep transient `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `.metareview/runs/` (FSM runs, self-ignoring) and `.metareview/shards/` (self-ignoring prompt packs) local unless the repository explicitly changes that contract. A `mock: true` FSM row never satisfies a gate.

In metaswarm repositories, use metareview to deepen metaswarm's existing review framework. Do not replace Beads task state, Superpowers workflows, or metaswarm PR shepherding.

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
