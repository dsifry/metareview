# metareview Architecture & Patterns

The map a new contributor (human or agent) should read first, so we stop reinventing wheels. It is a
**reference, not a tutorial** — it points at the deeper docs rather than duplicating them. Keep it current
when the shapes below change.

- Getting started / install: [INSTALL.md](../INSTALL.md), [quickstart](quickstart.md)
- Driving the FSM: [docs/fsm/driving-a-workflow.md](fsm/driving-a-workflow.md)
- Per-harness setup: [README.claude.md](README.claude.md) · [README.codex.md](README.codex.md) · [README.pi.md](README.pi.md)
- Review lenses: `rubrics/*.md`

---

## 1. What metareview is

A **review harness and quality-gate engine for coding agents.** It runs adversarial reviews over artifacts,
code diffs, PR readiness, and post-merge learning; records durable Markdown review logs + JSONL state; and
enforces **review-before-push** through a git-native gate (fail-closed;
`git push --no-verify` is the deliberate escape hatch; a pushed ref that is not the checked-out branch is
blocked rather than silently waved through — see §5). It runs standalone or as a deeper review layer
inside a metaswarm/Beads/Superpowers repo. Distributed as an npm package and as Claude Code / Codex plugins.

**Division of labor (remember this).** Deterministic machinery runs *outside* the model — anywhere, including
git hooks and CI — and emits exit codes + messages. The **agent** reads those, runs the actual adversarial
review (as subagent lenses), records findings/coverage, and lets metareview adjudicate. A hook or the CLI
gate never tries to run the agentic review itself.

## 2. The review types — nested surface areas

Each is a distinct scope, smaller-to-larger, **not** four ways to do the same thing. The **CLI** column is
abbreviated — task-done/epic-ready/pr-ready also take `--evidence <file>` (a validation receipt from
`evidence run`) and other flags; run `metareview review <type> --help` for the full form.

| Type | CLI | Reviews | Engine today |
|------|-----|---------|--------------|
| **artifact** | `review artifact <path>` | a spec/plan/design/doc (no code yet) | **agent lenses** (scaffold → subagents fill rows; `NOT_REVIEWED` until done) |
| **task-done** | `review task-done <id> --base <ref>` | one task's diff (small) | deterministic-local gate ⚠️ |
| **pr-ready** | `review pr-ready --base <ref>` | the whole branch diff (sharded over 120 KB) | deterministic-local gate ⚠️ |
| **epic-ready** | `review epic-ready <id>` | the epic's **integration diff** (base..HEAD, the union of the children's changes), **with the roll-up as context** — child evidence present? contradictions? intent drift? registry coverage? | deterministic heuristics (roll-up freshness) **+ a required adjudicated review** over the integration diff (base..HEAD) via the `epic-review-loop` workflow — same require-lenses gate as pr-ready/task-done |
| **learn** | `learn --post-merge <pr>` | what the merged PR + bot findings teach us | learning extraction |

`epic-ready` runs *after* every child task is task-done-reviewed. It reviews the epic's **integration diff**
(base..HEAD, the union of the children's changes) **with the roll-up — child review logs, evidence, parent
intent — as context**; the roll-up's own freshness is guarded by the deterministic pre-checks (re-read every
run). See `internal/reviewers/epicready.go`, `rubrics/epic-ready-review-rubric.md`.

## 3. Two review engines — and the gap between them

There are **two** engines that produce a review, joined by a **review-evidence marker**:

1. **Deterministic gate** (`review pr-ready` / `task-done`) — `ExecutionMode: deterministic-local`. Its
   "reviewers" are **structural checks**: validation evidence present, no unresolved prior blockers, clean
   working tree, mutation report clean, PR-evidence section readable. **No model call.** Fast, reproducible,
   runs in a git hook / CI. On its own it reports PASS for *structurally clean*, which is **not the same as
   *adversarially reviewed*** — so it no longer stands alone (see the require-lenses gate below).

2. **FSM review-lenses** (`metareview fsm --workflow …`) — the `review-lenses` node runs the adversarial
   lenses as **subagents with a real `$REVIEWER` model**, then `match-then-adjudicate` judges them. This is
   the genuine AI adversarial review. Its findings live in the **FSM run store**
   (`.metareview/runs/<id>/audit.jsonl`, carried as `FoldState.Findings`) — **not** in the
   `.metareview/findings.jsonl` that the deterministic gate reconciles and reads.

**The bridge (require-lenses gate).** `pr-ready`/`task-done`/`epic-ready` now **require** an adjudicated lens
review by default. After a real review the agent records a **review-evidence marker** —
`metareview review record-lenses --scope <pr-ready|task-done|epic-ready> --lenses <names> [--from-run <fsm-run-id>]` — a record in
`.metareview/runs.jsonl` (`scope="review-evidence"`, `Kind="review-evidence"`) carrying the adjudicated
verdict, confirmed finding IDs, lens set, execution mode, and the **base..HEAD SHAs it reviewed**. The gate
(`internal/reviewers/adversarial.go`) looks up the latest marker for the scope over the **exact
base..HEAD diff** via `reviewstate.LatestReviewEvidence`; a marker for a stale HEAD *or a different base*
does not count (a review of a narrow `HEAD~1..HEAD` must not be credited for a wider `main..HEAD`). It blocks
with `adversarial-review-reviewer` when no current marker is present, blocks when the adjudicated verdict is
not `PASS`/`PASS_ADVISORY`, and emits an **advisory** finding (not a block) when the marker is
`in-session-emulated` rather than `subagent-adjudicated`. Of several markers over one base..head, the
**last-recorded** wins (append order = record order), so a re-review's newer verdict supersedes the older.

The FSM stays scope-agnostic: the **agent** bridges its run into a marker with `--from-run`, rather than the
FSM emitting scope-specific markers. Because a CLI seam cannot witness that independent subagents actually
ran, `record-lenses --mode subagent-adjudicated` is admitted **only** when `--from-run` names an FSM run that
reviewed the same `base..head` (its init) AND reached a passing terminal transition (`clean|reviewed|fixed`);
an empty, wrong-diff, incomplete, or failed run is rejected, and a self-attested review has no such run and
must record the labeled, advisory `in-session-emulated` mode. This keeps a hand-typed one-liner from
laundering a fake review as full-strength independent evidence. A marker attests the committed `base..HEAD`
only, so a `--include-working-tree` run over a dirty tree blocks on a `working-tree-unattested` reviewer
despite a valid marker.

**Escape hatch.** `METAREVIEW_ALLOW_MECHANICAL_PASS=1` opts a single run out of the requirement, restoring
the old deterministic-only pass (`reviewstate.RequireAdjudicatedReview()`). `artifact` review is unchanged —
it still scaffolds `NOT_REVIEWED` and blocks until the agent fills real reviewer rows.

## 4. The FSM — a scope-agnostic review/fix loop

`metareview fsm` drives workflows built from **node kinds**, not review-type states (`internal/fsm/`,
`workflows/*.yaml`, [docs/fsm/](fsm/driving-a-workflow.md)):

- `review-lenses` (`exec: subagent, model: $REVIEWER`) — run the adversarial lenses over a diff/target.
- `match-then-adjudicate` (`exec: fork, model: $JUDGE`) — the judge adjudicates candidate findings.
- `agent-edit` — the fix.
- `still-present` — verify a finding is gone.
- `prove` — mutation-verify a fix (sdlc-loop-proved).

Workflows: `review-loop` (discover → adjudicate → done: a one-shot review, no fix loop), `sdlc-loop`
(discover → adjudicate → fix → verify), `sdlc-loop-clean` (adds `recheck` — **re-review the fix**, loop until
a fresh review is clean; the loop MUST target a review node), `sdlc-loop-proved` (adds `prove`). The loop is
defined by the *graph*, not flags; a loop reset clears findings.

epic-ready is **not** a fix-loop: it reviews child *readiness*, which has no "find-bug-fix-in-loop" shape. Its
adjudicated review is a **one-shot** `epic-review-loop` (a `review-loop` variant) the agent drives over the
epic's **integration diff** (base..HEAD, the union of the children's changes); its `discover` node carries a
`rubric: rubrics/epic-ready-review-rubric.md` param so the epic lenses (integration, acceptance-vs-intent,
cross-child regression, architecture coherence) are configured independently of the pr-ready/task-done lens
set — the `review-lenses` node's rubric is a per-workflow param defaulting to the task-done rubric.
**Division of labor:** the marker attests the integration-diff review (base..HEAD currency); the roll-up's
own freshness (child logs/evidence/intent, which live outside the diff) is guarded by the deterministic
pre-checks in `RunEpicReady`, which re-read current state on every gate run.

Exit contract (`metareview fsm`): `3` = the FSM needs the host to do a node's work; `1`+`GATE_FAILED` = run
`resume_hint` (forks a child = new run id); `1`+`ERR_*` = read `code`; `2` = nothing recorded (fix input and
retry unless it's a consent/escalation code); `STOPPED`/`DONE` terminal. Escalation is per fork lineage.

## 5. The git-native gate

Enforces review-before-push **in git**, not in a command-string parser (which is fundamentally bypassable —
`git commit;git push`, subshells, aliases, eval). See [INSTALL.md](../INSTALL.md) and `hooks/git/`,
`internal/setup/hooks.go`, `internal/githooktest/`.

- **pre-push** = the HARD gate: runs `review gate --push` (deterministic), BLOCKS an unreviewed/unresolved
  branch, **fails closed**; `git push --no-verify` is the escape hatch. It forwards git's pushed-ref stdin to
  the gate (`--pre-push-stdin`) and gates the **pushed refs**: a ref whose local sha equals the checked-out
  HEAD is gated as that branch (the common `git push` / `git push origin HEAD`); a **non-checked-out** ref
  (`git push origin other:main`, `HEAD~3:main`) is **BLOCKED** — the gate measures the checkout and cannot
  verify a different ref's content, so it fails closed with a check-it-out-or-`--no-verify` remedy (issue #82,
  Option B). A pure ref *deletion* is skipped. (Reviewing a non-checked-out ref's *own* content — so a reviewed
  cross-ref push passes without `--no-verify` — is the Option A follow-up.)
- **post-commit** = NEVER blocks (a commit saves work). Names the files it wrote + a review-owed nudge.
- **Commit-always, enforce-at-push:** saving work must never be held hostage to the reviewer being down;
  the enforcement lives at push, where not-pushing loses nothing.
- **Install materializes** (`setup --install-hooks`): the scripts are `go:embed`ded (root `githookassets.go`)
  and written into `<repo>/.metareview/git-hooks/` (executable, git-ignored, per-clone), with `core.hooksPath`
  pointed there and verified before "active" is reported. This is what makes the gate work in **any** repo,
  not just metareview's own checkout. Legacy `hooks/git` is reclaimed only when it carries metareview's
  content marker (or is entirely absent = broken-install recovery).

## 6. State, evidence & storage

- **Durable, committed** under `docs/metareview/`: review logs (`reviews/`), context packs (`context/`),
  shard results (`shards/`), FSM export bundles (`fsm/`), findings render (`FINDINGS.md`). ⚠️ Context packs
  can leak an absolute `cwd` (issue #80) — do not commit a leaking context artifact; the review `.md` is
  clean.
- **Transient, local (git-ignored)** under `.metareview/`: `findings.jsonl`, `runs.jsonl`, `runs/`,
  `shards/`, `git-hooks/`. A `mock: true` FSM run never satisfies a gate.
- **Run lineage:** a NEEDS_REVISION parent is retired when a clean same-target+same-kind child links via
  `previousRunId` (supersede). Repair via `--previous-run <run-id>`; never `git add -A` failed-run artifacts.
- **Evidence receipts:** `evidence run -- <cmd>` records a validation receipt (kind + exitCode + hashes);
  `evidence import --github-checks <pr>` pulls CI. task-done/pr-ready require a passing validation receipt.
- **Sharded review** (exclude-filtered diff > 120 KB): the gate writes prompt packs under
  `.metareview/shards/…/plan.json`; review one subagent per shard + a cross-shard pack, write results, re-run
  with `--previous-run`. Editing a file invalidates only its own shard.
- **Overrides** (`override request` / `grant`): requesting does NOT clear the gate; granting must come from
  **outside** the workflow (a human/authority) — the requester cannot grant. `--by` is audit metadata, not
  authentication. An override is never a fix (`fixedInRunId` stays empty).

## 7. Cross-agent integration

The portable core is the **Agent Skills standard** (`skills/<name>/SKILL.md` with `name`/`description`
frontmatter) — metareview already ships it, and it serves **Claude Code**, **Codex**, and **Pi** with the
same files. MCP is the *second* surface for the MCP-speaking long tail (OpenCode, Cursor, Cline, Zed) but
**cannot reach Pi**, which is CLI+skill by design. So: Skills reach Pi & co.; an MCP server (not built yet)
reaches the GUI/TUI agents; thin per-agent command wrappers on top.

- **Claude Code / Codex:** `.claude-plugin/` + `.codex-plugin/` manifests; marketplace is the metareview
  repo itself (`.claude-plugin/marketplace.json`, which Codex accepts as legacy-compatible). Skills → `/name`
  (Claude) or `$name` (Codex).
- **Pi** (`github.com/earendil-works/pi`): point it at the skills under `.agents/skills/` or
  `~/.agents/skills/`; invoke `/skill:name`. No manifest needed.
- **metaswarm** is a *separate* installable orchestration
  (**[github.com/dsifry/metaswarm](https://github.com/dsifry/metaswarm)**), not metareview. `setup --check`
  also detects a local sibling checkout at `../metaswarm` for development convenience, but the canonical
  source is the repo URL. In a metaswarm repo, metareview is the deeper review gate; do not replace Beads
  task state or metaswarm PR shepherding.

## 8. Package map

Grouped by role (mostly `internal/`; not exhaustive — `run ls internal/` for the full set, and the `fsm/`
list below is illustrative, omitting e.g. `judge`, `gate`, `converge`, `export`):

- **Entry / dispatch:** `cmd/metareview` (CLI), `internal/repo` (root detection), `internal/version`.
- **Review types:** `artifactreview`, `taskdone`, `prready`, `epicready`, `reviewers` (the deterministic
  reviewer lenses), `lens` (the lens set).
- **Review state & logs:** `reviewlog` (parse/discover `.md` logs), `reviewstate`, `reviewmanifest`,
  `findings`, `runchain` (lineage), `state`/`jsonl` (append/scan), `reviewprompt`.
- **Gate & install:** `setup` (mode/prereqs + hook install), `status` (branch scope, `CommitGate`/`PushGate`,
  `BuildForBranch`, coverage/unreviewed), `githooktest` (black-box hook tests), `covergate` (floor gate).
- **Context & evidence:** `gitcontext` (exclude-filtered diff), `githubcontext`, `contextpack`,
  `contextprofile`, `shardpack` (shard packs), `evidence`, `mutation` (Stryker/gremlins report), `knowledge`,
  `markdown`, `classify` (file class), `testconv` (test-file convention).
- **FSM:** `fsm/{cli,machine,workflow,run,record,sandbox,kind}`.
- **Sources & learning:** `tasksource`, `epicsource` (Beads etc.), `learning`, `learnsource`,
  `sessionhistory`, `integration` (metaswarm).

## 9. Build process & conventions

- **TDD, dependency injection, mock-AI, enforced coverage.** Two distinct mechanisms (`Makefile` `cover`,
  `internal/covergate`): (a) a dynamically-generated **require-100 set** — `go list ./internal/fsm/... ./workflows`
  — is held at exactly 100% of statements and is **not** listed in the floor file; (b) every *other* package
  has a **ratcheting per-package floor at its measured level** in `tests/coverage-floor.txt` (most are well
  below 100 — e.g. `internal/knowledge 77.8`), which `--update-floor` raises but never lowers. So a new
  non-FSM package needs a floor line; a new `internal/fsm/...` package is auto-required at 100% instead.
  `make cover` / `tests/coverage.sh` merge unit + a behavioral shell suite via `go tool covdata`. A package
  with **no statements** stays out of the profile (e.g. the embed-only root package) — don't floor it.
- **The command-seam DI pattern:** git access goes through an injectable `RunGit`/`GitRunner` func so logic is
  hermetically testable without a real repo; `nil` uses the real binary. Mirror it for any external command.
- **Embed + materialize:** ship scripts/templates via `go:embed`, write them into the target on demand,
  verify before claiming success (see the hook install). Don't assume files exist on disk in a consumer repo.
- **Fail closed:** when the gate can't tell (unresolvable scope, unreadable state, a git error listing what a
  commit writes), block — never wave through.
- **Mutation testing (gremlins):** the reliable, complete-verdict config is `--workers 1 --timeout-coefficient
  30`; `--workers 8 --timeout-coefficient 120` is a faster config with a few flaky timeouts. Timeouts are
  recompile contention, not real survivors. 100% line coverage still leaves killable mutants — construct the
  distinguishing test before calling a survivor equivalent.
- **Review-first, then bots:** run metareview's adversarial review BEFORE opening the PR, so CodeRabbit/Cursor
  measure only the *residual* we missed — the recall yardstick. Reviewing after the PR opens confounds it.

## 10. Gotchas that have bitten us

- A single-package test fixture hides multi-package `go test ./...` classification bugs — key on the target's
  own `-v` markers.
- A mock judge ignores model/effort — a node that calls the judge needs `model` in the YAML or it's DOA in
  prod.
- Differential proof binds against `base..head`, not the fix diff — bind against `FixEntryHead..head`.
- task-done scans untracked files: a finding's own TODO text can self-reference; clear via `--previous-run`
  to the opening run, and an untracked file over 4,000 bytes raises `UNTRACKED_TRUNCATED`.
- The module-root package's coverage label differs between `coverage.sh` and `make cover` — keep it
  statement-free so it's out of the profile.
