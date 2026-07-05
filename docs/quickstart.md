# metareview Quickstart

For full installation paths, see [`../INSTALL.md`](../INSTALL.md). For coding-agent instructions, see [`README.codex.md`](README.codex.md), [`README.claude.md`](README.claude.md), [`../AGENTS.md`](../AGENTS.md), and [`../CLAUDE.md`](../CLAUDE.md).

## 1. Check Mode

Run from the repository root:

```bash
metareview setup --check
```

For standalone use, inspect the dry-run prerequisite plan:

```bash
metareview setup --bootstrap-prereqs --dry-run
```

The dry run does not install Superpowers, Beads, or metaswarm. Non-dry-run bootstrap requires explicit confirmation.

## 2. Capture Validation Evidence

Prefer structured receipts over prose evidence:

```bash
tmp_evidence="$(mktemp)"
metareview evidence run -- go test ./... > "$tmp_evidence"
metareview evidence run -- git diff --check >> "$tmp_evidence"
```

After a GitHub PR exists, CI checks can be imported into the same evidence file:

```bash
metareview evidence import --github-checks <pr-number> --repo <owner/repo> >> "$tmp_evidence"
```

Freeform evidence files still work as a fallback, but receipts preserve command, working directory, exit code, timestamps, summary, and output hashes. Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals.

## 3. Run Reviews At The Right Gate

Use the smallest gate that matches the work:

```bash
metareview review artifact <path>
metareview review task-done <task-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
metareview review pr-ready --base <base-ref> --evidence "$tmp_evidence"
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

`artifact` creates an incomplete review scaffold for specs, plans, and docs. The command exits nonzero while the scaffold is still `NOT_REVIEWED`; complete every required reviewer row and update the verdict before treating the artifact as reviewed. Artifact review runs the five required lenses as parallel subagents by default. Use `in-session-emulated` only when subagents are unavailable or the human explicitly requests no delegation, and state that the review is not independently adversarial and is weaker evidence. Use `--scaffold-only` only when scaffold creation itself is the intended action. `task-done` runs after a local task or chunk claims done. `epic-ready` runs when child tasks are complete. `pr-ready` runs before push or merge readiness. `learn --post-merge` runs after confirmed PR merge.

Lifecycle gate results use this contract:

- `PASS`: proceed.
- `PASS_ADVISORY`: proceed only when the review reports zero blocking findings.
- `NEEDS_REVISION`: fix blockers, then re-run the same gate with `--previous-run <run-id>`.
- `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.

Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr. `NOT_REVIEWED` artifact scaffolds are also blocking until completed.

Task-done, epic-ready, and PR-ready context packs now include a Context Profile and Context Shard Plan when risk requires sharding. Task-done and PR-ready also include a Review Manifest that accounts for source paths, generated path dispositions, shard assignments, manifest hashes, and manifest blockers.

## 4. Metaswarm Fit

When metaswarm, Superpowers, and Beads are present, metaswarm remains the lifecycle owner. Metareview supplies deeper review commands and durable artifacts. The integration contract is in `docs/integrations/metaswarm.md`.

In standalone mode, metareview still runs advisory reviews and can use `.metareview/knowledge/metareview.jsonl` until Beads knowledge is available.

## 5. What To Commit

Commit:

- `docs/metareview/reviews/`
- `docs/metareview/context/`
- `docs/metareview/learning/`
- `.metareview/knowledge/metareview.jsonl` in standalone fallback mode
- `.metareview/calibration.jsonl`
- `.metareview/learning-runs.jsonl`
- `.beads/knowledge/metareview.jsonl` when Beads exists

Keep local:

- `.metareview/findings.jsonl`
- `.metareview/runs.jsonl`
- other transient `.metareview/` state

For ordinary project repositories, use exact file entries for transient state. Do not ignore `docs/metareview/` or the entire `.metareview/` directory, because those patterns hide durable review, learning, calibration, or fallback knowledge artifacts.

```gitignore
.metareview/findings.jsonl
.metareview/runs.jsonl
```

The repository `.gitignore` keeps transient state local while allowing fallback learning knowledge and calibration to sync through git.

## 6. Agent Syntax

Codex users invoke metareview through `$setup`, `$review-artifact`, `$review-task-done`, `$review-epic-ready`, `$review-pr-ready`, `$learn-post-merge`, and `$status`.

Claude Code users invoke the same workflows through `/setup`, `/review-artifact`, `/review-task-done`, `/review-epic-ready`, `/review-pr-ready`, `/learn-post-merge`, and `/status`.

Direct CLI usage remains the source of truth when plugin skills are unavailable.
