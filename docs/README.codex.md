# Codex CLI Integration

metareview supports Codex as a plugin and as a direct CLI.

## Install

```bash
codex plugin marketplace add dsifry/metareview-marketplace
codex
```

Open `/plugins`, select the metareview marketplace, and install `metareview`.

For local development from a checkout:

```bash
codex plugin marketplace add /path/to/metareview
codex
```

## Skill Syntax

Codex uses `$skill` syntax:

| Skill | Purpose |
| --- | --- |
| `$setup` | Detect repository mode and prerequisites. |
| `$review-artifact` | Review specs, plans, docs, designs, and decompositions. |
| `$review-task-done` | Run task-done review before claiming local code work complete. |
| `$review-epic-ready` | Check parent readiness after child tasks are complete. |
| `$review-pr-ready` | Check PR readiness before push or merge. |
| `$learn-post-merge` | Extract post-merge learning after a PR merges. |
| `$status` | Show current review state. |

## Direct CLI Fallback

Use direct commands when a skill is unavailable:

```bash
metareview setup --check
metareview review artifact <path>
metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
metareview review epic-ready <epic-id-or-path>
metareview review pr-ready --base <base-ref>
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

In a source checkout without a packaged binary, prefix commands with `go run ./cmd/metareview`.

## Agent Contract

Codex agents must not claim work complete while a blocking finding remains open or while an artifact review remains `NOT_REVIEWED`. The default artifact command exits nonzero after scaffold creation until agents complete the required reviewer rows and final verdict. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence. Fix blockers, re-run with `--previous-run <run-id>`, and proceed only after `PASS` or `PASS_ADVISORY` with zero blockers.

Commit durable review artifacts under `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.

## Metaswarm Repositories

When metaswarm is installed, keep using metaswarm and Beads as the lifecycle source of truth. Insert metareview as the deeper review gate for artifact, task-done, epic-ready, pr-ready, and post-merge checkpoints.
