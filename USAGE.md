# Using metareview

metareview is a local-first review and repair harness for humans and coding agents. It does four kinds of
work, and this guide is organized around them:

1. **[Review before you commit](#1-review-before-you-commit)** — named gates for specs, plans, code,
   epics, and PRs, with deterministic blockers and durable Markdown evidence.
2. **[Find and fix bugs — with proof](#2-find-and-fix-bugs--with-proof)** — an audited state-machine loop
   that discovers defects, confirms them with a judge, fixes them, and makes each fix carry a
   *differential proof* that actually holds.
3. **[Drive critical code to exhaustive tests](#3-drive-critical-code-to-exhaustive-tests)** — coverage and
   mutation tooling so the pieces that matter are comprehensively, not superficially, tested.
4. **[Learn from what merged](#4-learn-from-what-merged)** — extract durable, git-native lessons.

Two cross-cutting things worth knowing up front: metareview is **model-swappable** (choose the judge —
Claude, Codex/GPT, GLM, Kimi — per run; see [Choosing a judge model](#choosing-a-judge-model)) and
**multi-language** (Go, TypeScript/Jest, Vitest, Python/pytest; see
[Multi-language projects](#multi-language-projects)).

> New here? `metareview setup --check` reports your repo mode and prerequisites. Install steps are in
> [INSTALL.md](INSTALL.md).

---

## 1. Review before you commit

Use the **smallest gate that matches the decision** you're making. Each gate produces a Markdown review
under `docs/metareview/` and returns a result you (or an agent) must honor.

| You're deciding… | Gate |
| --- | --- |
| Is this spec/plan/design/decomposition good enough to build from? | `review artifact <path>` |
| Is this task-sized code change actually done? | `review task-done <id-or-path> --base <ref> --evidence <file>` |
| Is this parent epic ready now that its children landed? | `review epic-ready <id-or-path> --base <ref> --evidence <file>` |
| Is this branch ready to push / open as a PR / merge? | `review pr-ready --base <ref> --evidence <file>` |

```bash
# capture validation evidence first (receipts with exit codes, timestamps, output hashes)
tmp="$(mktemp)"
metareview evidence run -- go test ./... > "$tmp"
metareview evidence run -- git diff --check >> "$tmp"

metareview review artifact docs/spec.md
metareview review task-done docs/tasks/task-001.md --base main --evidence "$tmp"
metareview review pr-ready --base main --evidence "$tmp"
```

**The result contract (memorize this):**

- `PASS` — proceed.
- `PASS_ADVISORY` — proceed only with **zero** blocking findings.
- `NEEDS_REVISION` — fix the cited blockers, then re-run the **same** gate with `--previous-run <run-id>`.
- `ESCALATED` — stop retrying the same target; a human must narrow, split, or redesign it.

Artifact review runs **nine adversarial lenses as parallel subagents** by default (feasibility,
completeness, scope/alignment, architecture, intent-preservation, security, testing-quality,
data-migration, mechanical-precision). A `NOT_REVIEWED` scaffold is *not* a pass.

**Big diffs are handled.** When a branch diff exceeds the review context limit, `task-done`/`pr-ready`
cut it into content-stable **shards** and write one prompt pack per shard; you review each pack and write
a result back, and the context-risk blocker clears once every shard (plus the cross-shard seam) has a
fresh passing result. See the sharded-review section in [CLAUDE.md](CLAUDE.md).

### Make it mechanical — the git-native gate

Prose ("review before you push") is not a gate. Install the **git-native hooks** and it is enforced by git
itself, so it holds however a command is spelled:

```bash
metareview setup --install-hooks        # interactive; --yes for headless, --dry-run to preview
```

- **`git push` is blocked** until the branch is review-clean — the `pre-push` hook runs `metareview review
  gate --push` (deterministic) and aborts the push on a nonzero result, *before* anything leaves. It fails
  closed; `git push --no-verify` is the escape hatch. The gate measures the checked-out branch; pushing a
  different ref is not yet gated ([#82](https://github.com/dsifry/metareview/issues/82)).
- **metareview never blocks a commit** — a commit saves work, and the `post-commit` hook cannot block one
  anyway (it runs after the commit). It just names the files it wrote and reminds you (and the agent) to
  review them before pushing.

The hooks run **outside** the agent, so they only run the deterministic gate and emit a message; the agent
reads that message, runs the review, records coverage (metareview adjudicates), and re-pushes. Install is
non-destructive and reversible (`--uninstall-hooks`); see [INSTALL.md](INSTALL.md).

---

## 2. Find and fix bugs — with proof

`metareview fsm` drives an **audited workflow state machine**. The flagship workflow, `sdlc-loop-proved`,
runs `discover → adjudicate → fix → prove → verify` and closes the loop only when the fix carries a proof
that holds:

- **discover** — adversarial reviewer lenses read the diff and propose findings.
- **adjudicate** — a judge confirms or rejects each finding (this is your false-positive guard).
- **fix** — the fixer edits the tree, commits, and **declares a differential proof** for each bug.
- **prove** — metareview runs that proof *deterministically* against the real test command:
  - a **reproduction** proof — a test that **fails on the pre-fix code with a real assertion** and
    **passes after** the fix;
  - a **pin** proof — a guard line the fix added, plus a still-compiling change that breaks it and a test
    that catches the break (mutation-verified);
  - a **deletion** proof — for a fix that removed faulty code.
- **verify** — a judge confirms the symptom is gone.

The point: a fix isn't trusted because an agent says so — it's trusted because a **deterministic proof**
demonstrates the bug was real and is now gone. `{commit, summary}` is not evidence; a proof is.

### Driving the loop

The driver contract is `metareview fsm --agent-prompt`. In short: `init` once, then repeat
`advance` → (on exit 3, do the node's work) → `record node-output` → `advance`. Nodes are `subagent`
(you spawn reviewers), `inline` (you do it in-session), or `fork` (the CLI runs the judge). Full guide:
[`docs/fsm/driving-a-workflow.md`](docs/fsm/driving-a-workflow.md).

```bash
metareview fsm init --workflow sdlc-loop-proved --base <clean-ref> \
  --judge-model <model> --judge-effort medium
metareview fsm state          # where the run is; follow next_action
metareview fsm advance         # take the next step
```

**Exit codes for `fsm`:** `3` = the host must do a node's work; `1 + GATE_FAILED` = run the `resume_hint`
(it forks a child with a new run id); `1 + ERR_*` = read `code` (`detail` is data); `2` = nothing
recorded, fix the input and retry unless it's a consent/escalation code (which waits for a human);
`STOPPED`/`DONE` are terminal. Runs are **forkable and resumable**, and every judge call is recorded in
the run's audit log and export bundle (`metareview fsm export`).

### Custom test commands and the consent gate

The loop runs your project's real test command to prove fixes. The first `init` with a non-default command
returns `ERR_CMDS_NOT_ALLOWED` with a `cmds_sha256` — **a human decision**: re-run with
`--allow-custom-cmds <sha>` to consent to exactly that command. This is deliberate: metareview will not run
an arbitrary command on your behalf without explicit consent.

> Run the loop on **throwaway clones** of repos you care about — it makes commits.

---

## 3. Drive critical code to exhaustive tests

Coverage tells you what *ran*; it does not tell you what's *tested*. metareview supports both.

- **Mutation reports.** metareview ingests a mutation tool's own structured output as review evidence, so
  a surviving mutant on a "100%-covered" line becomes an actionable finding:
  `metareview review task-done <target> --base <ref> --mutation-report <file>` (gremlins JSON for Go; the
  Stryker schema for TS; mutmut for Python). A run whose mutation summary is dishonest (e.g. timeouts
  scored as kills) is refused rather than trusted.
- **Coverage gate.** The repo ships a Go-native coverage gate (`make cover`) that holds critical packages
  at 100% of statements and floors the rest, so coverage can only ratchet up.

The workflow for hardening a package is: cover the uncovered statements with **meaningful** tests (not
line-execution filler), then run a mutation pass and write a killing test for every survivor. See
[docs/qa-shakedown-runbook.md](docs/qa-shakedown-runbook.md) for the escalation ladder (wiring smoke →
recall of planted bugs → real historical bugs → hidden-gold review → mutation survivors).

---

## 4. Learn from what merged

After a PR is confirmed merged, extract durable lessons into local, git-native knowledge:

```bash
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

Unlike hosted reviewers, this knowledge stays **local, non-proprietary, and human-readable** (Markdown /
JSONL under `docs/metareview/`), so future reviews start smarter — repeated mistakes, idiosyncratic repo
decisions, and reviewer calibration — while stale entries are pruned as the code evolves.

---

## Choosing a judge model

Every judge call (adjudicate / verify / the symptom reviewer) is **swappable per run** and recorded in the
audit, so the model that judged a run stays visible in its snapshot and export.

```bash
# Claude (default family) or Codex/GPT via the Codex CLI OAuth session (no API key needed):
metareview fsm init … --judge-model claude-opus-4-8
metareview fsm init … --judge-model codex/gpt-5.6-sol

# OpenAI-compatible providers (GPT, GLM, Kimi) via an endpoint you point at:
OPENAI_BASE_URL="https://your-openai-compatible-host"  OPENAI_API_KEY="your-api-key" \
  metareview fsm init … --judge-model glm-5.3-flash
```

`--judge-model` (or `METAREVIEW_JUDGE_MODEL`) retargets the judge without editing the workflow. Models are
routed by name: `claude*`/`anthropic/*` → the Anthropic API; `codex/*` → the Codex CLI; `gpt*`,
`openai/*`, `glm*`, `kimi*` → an OpenAI-compatible `/v1/chat/completions` endpoint (`OPENAI_BASE_URL`).
Reasoning models (e.g. GLM) are given a generous token budget automatically. Compare two runs' verdicts
with `metareview fsm diff --a <run> --b <run>`.

---

## Multi-language projects

The differential-proof engine is language-agnostic; the language-specific bit is a small **test
convention** that reads each runner's own machine-readable output — never a bespoke parser:

| Language | `test_convention` | Reads |
| --- | --- | --- |
| Go (default) | `go` | `go test -json` (test2json) |
| TypeScript / Jest | `typescript` | `jest --json` |
| TypeScript / Vitest | `vitest` | `vitest run --reporter=json` |
| Python / pytest | `python` | pytest JUnit XML (`-o junit_family=xunit1 --junit-xml=/dev/stdout`) |

The shipped `sdlc-loop-proved` workflow defaults to Go. For another language, point the `prove` node's
`test_convention` at the right value and set the workflow's test command to your runner (a per-language
workflow variant). The runbook, [docs/qa-shakedown-runbook.md](docs/qa-shakedown-runbook.md), has copy-ready
variants and a Docker-isolated toolchain pattern so you don't install runners locally.

> Zero-config auto-detection of language + test command (so a mixed-language repo just works) is a
> designed-and-specced next step; today, non-Go repos use a workflow variant.

---

## Overrides and escalation

A blocking finding is normally cleared by fixing it. When the workflow is deliberately stepped outside of,
record it rather than working around it:

```bash
metareview override request <finding-id> --reason "<why the workflow was exited>"
metareview override grant   <finding-id> --reason "<why the exception is accepted>"
metareview override list [--pending]
```

**Requesting** keeps the gate red (CI stays failing); **granting** is the acknowledgement and must come
from *outside* the workflow — the actor who requested an override cannot grant it. Both halves are recorded
with actor, timestamp, and reason, and an override is never a fix — so post-merge learning can analyze
exceptions separately from resolutions.

---

## Where output lives

- **Durable, commit these:** Markdown reviews and context under `docs/metareview/` (including FSM export
  bundles under `docs/metareview/fsm/` and shard results under `docs/metareview/shards/`).
- **Transient, keep local:** `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, FSM runs under
  `.metareview/runs/` (self-ignoring), and shard prompt packs under `.metareview/shards/` (self-ignoring).

Ignore only the two transient files by exact name — never the whole `docs/metareview/` or `.metareview/`
directories:

```gitignore
.metareview/findings.jsonl
.metareview/runs.jsonl
```

---

See [README.md](README.md) for the overview, [INSTALL.md](INSTALL.md) for installation, and
[CLAUDE.md](CLAUDE.md) for the agent-facing operating contract.
