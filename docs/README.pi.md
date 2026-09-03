# Pi Integration

metareview works with [Pi](https://github.com/earendil-works/pi) (the terminal coding agent) through the
**Agent Skills** standard — the same `SKILL.md` skills metareview already ships for Claude Code and Codex.
Pi has no MCP by design; its intended model is *exactly* metareview's: a CLI plus a skill that documents it.

## Install

Install the CLI, then make metareview's skills discoverable to Pi.

```bash
npm install -g metareview           # or use a source checkout with Go 1.26+ (go run ./cmd/metareview ...)
```

Pi auto-discovers skills from `.agents/skills/` (the current repo and its ancestors up to the git root) and,
globally, from `~/.agents/skills/`. Point either at metareview's bundled skills — each is a
`<skill>/SKILL.md` directory, which is exactly what Pi expects. Pick **one** of the two placements below
(using both puts the same skill names in two locations, which Pi warns about and resolves by keeping the
first found):

```bash
# Per-repo (recommended): symlink each metareview skill into this repo's .agents/skills/
mkdir -p .agents/skills
for s in "$(npm root -g)"/metareview/skills/*/; do
  ln -s "$s" ".agents/skills/$(basename "$s")"
done

# Or globally, for every repo (Pi reads grouping folders under ~/.agents/skills/):
mkdir -p ~/.agents/skills
ln -s "$(npm root -g)/metareview/skills" ~/.agents/skills/metareview
```

From a source checkout, symlink `./skills/*` instead of the npm path. The per-repo flat form above is the
most portable; if your Pi version discovers a different layout, check its skills docs.

## Skill Syntax

Pi invokes skills as `/skill:<name>` (append arguments after a space):

| Skill | Purpose |
| --- | --- |
| `/skill:setup` | Detect repository mode and prerequisites. |
| `/skill:review-artifact` | Review specs, plans, docs, designs, and decompositions. |
| `/skill:review-task-done` | Run task-done review before claiming local code work complete. |
| `/skill:review-epic-ready` | Check parent readiness after child tasks are complete. |
| `/skill:review-pr-ready` | Check PR readiness before push or merge. |
| `/skill:learn-post-merge` | Extract post-merge learning after a PR merges. |
| `/skill:status` | Show current review state. |
| `/skill:fsm` | Drive a workflow run (sdlc-loop, review-loop) as an audited state machine. |

## Direct CLI Fallback

The skills call the CLI; you can run it directly when a skill is unavailable:

```bash
metareview setup --check
metareview evidence run -- go test ./... > /tmp/metareview-evidence.jsonl
metareview review artifact <path>
metareview review task-done <task-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
metareview review pr-ready --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

In a source checkout without a packaged binary, prefix commands with `go run ./cmd/metareview`.

## The Git-Native Review Gate

The review gate is enforced by git, not by Pi, so it holds no matter which agent (or a human) runs the push.
Install it once per clone:

```bash
metareview setup --install-hooks        # interactive; --yes for headless
```

It materializes a `pre-push` gate (blocks an unreviewed push; `git push --no-verify` escapes) and a
`post-commit` review-owed nudge into `.metareview/git-hooks/`. See [INSTALL.md](../INSTALL.md).

## Agent Contract

Do not claim work complete while a blocking finding remains open or an artifact review is `NOT_REVIEWED`.
Gate results are actionable: `PASS`/`PASS_ADVISORY` proceed only with zero blockers; `NEEDS_REVISION` repairs
via `--previous-run <run-id>`; `ESCALATED` stops same-target retries (a human must narrow, split, or redesign
the target). Exit handling: `0` verify a passing verdict; `1` with a review path means follow that log;
nonzero without a path means read stderr. Commit durable review artifacts under `docs/metareview/`; keep
transient `.metareview/` state local.
