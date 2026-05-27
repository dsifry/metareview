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

## 2. Run Reviews At The Right Gate

Use the smallest gate that matches the work:

```bash
metareview review artifact <path>
metareview review task-done <task-id-or-path>
metareview review epic-ready <epic-id-or-path>
metareview review pr-ready --base <base-ref>
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

`artifact` creates an incomplete review scaffold for specs, plans, and docs. The command exits nonzero while the scaffold is still `NOT_REVIEWED`; complete every required reviewer row and update the verdict before treating the artifact as reviewed. Artifact review runs the five required lenses as parallel subagents by default. Use `in-session-emulated` only when subagents are unavailable or the human explicitly requests no delegation, and state that the review is not independently adversarial and is weaker evidence. Use `--scaffold-only` only when scaffold creation itself is the intended action. `task-done` runs after a local task or chunk claims done. `epic-ready` runs when child tasks are complete. `pr-ready` runs before push or merge readiness. `learn --post-merge` runs after confirmed PR merge.

If a review reports any blocking finding or remains `NOT_REVIEWED`, fix it and re-run with `--previous-run <run-id>` until the result is `PASS` or `PASS_ADVISORY` with zero blockers.

## 3. Metaswarm Fit

When metaswarm, Superpowers, and Beads are present, metaswarm remains the lifecycle owner. Metareview supplies deeper review commands and durable artifacts. The integration contract is in `docs/integrations/metaswarm.md`.

In standalone mode, metareview still runs advisory reviews and can use `.metareview/knowledge/metareview.jsonl` until Beads knowledge is available.

## 4. What To Commit

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

The repository `.gitignore` keeps transient state local while allowing fallback learning knowledge and calibration to sync through git.

## 5. Agent Syntax

Codex users invoke metareview through `$setup`, `$review-artifact`, `$review-task-done`, `$review-epic-ready`, `$review-pr-ready`, `$learn-post-merge`, and `$status`.

Claude Code users invoke the same workflows through `/setup`, `/review-artifact`, `/review-task-done`, `/review-epic-ready`, `/review-pr-ready`, `/learn-post-merge`, and `/status`.

Direct CLI usage remains the source of truth when plugin skills are unavailable.
