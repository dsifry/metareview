# A lights-out software factory on the metareview FSM

**Status:** reviewed, revision 10 (after nine artifact-review rounds; see §15) · **Date:** 2026-09-04
(rev 6: 2026-09-05) · **Builds on:** issue #2 (*epic: make metareview verdicts stateful, shardable, and
evidence-backed*), `docs/ARCHITECTURE.md`, the 0.9.0 FSM specs.

**Supersedes in part (declared, see §13):** (a) the "review gates only, metaswarm owns the lifecycle"
boundary in `docs/integrations/metaswarm.md`, `CLAUDE.md` *Durable Output*, `AGENTS.md` *Metaswarm Fit*,
`docs/ARCHITECTURE.md` §7, and the memory note *metareview-decompose-build-flow*; (b) `ARCHITECTURE.md`
§3's rule that "the FSM stays scope-agnostic; the agent bridges its run into a marker" — the engine writes
the review-evidence marker for a child run it verified; (c) the engine's "every failure is terminal,
recovery is a fork" error model (0.9.0 core spec §5.1) — a non-terminal *await*, a retryable error class,
routed convergence, and a machine-level cancel are added; (d) `docs/fsm/driving-a-workflow.md`'s caveat
that the audit chain is "process guarantees for a cooperating agent" — the factory's host is not a
cooperating agent, so §5.12 adds a trust boundary.

> **Goal.** A *lights-out software factory*: given an intent (an issue, a spec, a plan, a one-line ask), the
> system designs → specifies → decomposes → builds → reviews → fixes → verifies → ships → closes → learns,
> **unattended up to a PR that is ready to merge**, with every invariant that metaswarm and Superpowers
> currently ask a model to honor turned into a machine-checked gate, and every "ask the human" moment
> turned into either a policy default or a typed escalation that parks durably and resumes in place.
> Merge itself always requires one act the factory cannot perform (§5.12.5; decision §12.8, accepted).
> "Deploy" is verified through runtime receipts from declared commands; the factory does not own a
> pipeline (§11).

---

## 0. Executive summary

Three systems were inventoried in full (§1). The finding that shapes everything else:

- **metaswarm** covers the whole lifecycle but is **entirely prompt-enforced**. Its 4-phase loop, its
  design/plan gates, its 3-retry counters, its pr-shepherd state machine are Markdown. Its own
  `docs/coverage-enforcement.md` calls the agent checklist "the weakest gate".
- **Superpowers** supplies the *method* (brainstorm → plan → SDD/TDD → review → finish) with ~70
  observable invariants — and **zero enforcement code**.
- **metareview** has the opposite profile: a hash-chained, replayable, consent-gated **deterministic FSM**
  with a real adjudicating judge, differential proofs, sharded review, evidence receipts, run lineage with
  escalation, overrides, and a git-native push gate — but it covers only the *review / fix / verify* third
  of the lifecycle. It authors nothing, schedules nothing, never runs `bd`, never writes to GitHub, never
  merges, never closes.

So the factory is **metareview's engine extended to carry the other two-thirds of the lifecycle**, with the
existing bug loop wrapped — not replaced — by a new `task-build-loop` (implement → engine-verified
TDD/scope/validation → `fix-loop-proved-clean` → mutation → marker) that is called *fractally*: an
`epic-build-loop` schedules dependency-free tasks into it and then runs the epic-level integration review;
a shared `finalize-loop` produces the epic-ready and pr-ready markers at the pushed head; a `factory`
loop wraps intake → design → decompose → epic-build → finalize → ship → release. metaswarm's and
Superpowers' rubrics, prompt recipes, and measured design lessons are reused as **node prompts and
rubrics**, not as the control plane.

Four review rounds fixed the design structurally: the engine's error model cannot carry a lights-out
escalation, so §5.3 adds `await`, a retryable error class, routed convergence with answer-re-armed
windows, cancel, and parse rules that make every reachable outcome routed; an autonomous host with shell
access in the tree the gates read can mint its own evidence — and a driver that runs `git` in, or checks
out content from, an agent-owned tree executes agent-controlled config and attributes — so §5.9/§5.12 put
every gate input, the `.git` directory, and every git operation on the agent side of a sandbox with a
one-way, config-free `fetch` as the only crossing; and prose outlines are not precise enough for this
engine, so §6 is gated edge lists against the grammar of §5.3 with every data channel named once.

The plan is eight epics across ten phase labels (0a, 0b, 1–4, 5a, 5b, 6, 7; 7a/7b are tracks within
Phase 7; §9). Phases 0–2 produce the fractal build unit and the epic loop; from Phase 2 on, each phase's
tasks are built by `task-build-loop` (§12). Issue #2's open items are absorbed (§10).

---

## 1. Sources and method

| Inventory | Source | Notes |
|---|---|---|
| metaswarm v0.12.0 | `../metaswarm` (242 files) | 14 skills, 16 commands, 19 agent personas, 9 rubrics, 6 guides, 22 templates, hooks, scripts |
| Superpowers v6.0.3 | `~/.claude/plugins/cache/claude-plugins-official/superpowers/896224c4b187/` | 14 skills + prompt templates + SDD scripts + the project's own measured design specs |
| metareview 0.10.1 | this repo @ `c0b933b` | every command/flag, node kind, gate, rubric, state file; `gh issue view 2 --comments` |

The facts that matter are restated here with citations so this spec stands alone. **Issue number:** the
planning epic is **#2** (metareview #1 is the closed `validation-reviewer` bug). **External verbs assumed but
not verified on this machine** (`bd`, `gtg`, `gemini` are not installed here): `bd ready/show/list/create
--json` are documented by metaswarm; `bd update/close/dep --json`, `bd sync` modes and auto-flush
behaviour, whether the `bd` database can be isolated per checkout, and `gtg --format json` must be pinned
to a version and verified during Phase 1 / Phase 4 decomposition before use.

---

## 2. The reference lifecycle we are replicating

| # | Stage | metaswarm mechanism | Superpowers mechanism | Deterministic today? |
|---|---|---|---|---|
| S0 | Session start / prime | `hooks/session-start.sh`: self-heal mandatory files, `bd prime` injection; `/prime --work-type …` | `session-start` hook injects `using-superpowers` (the 1 % rule) | partly (shell) |
| S1 | Entry / triage | `/start-task`: recovery check, Simple/Complex assessment (human confirms; Simple skips design/plan), GitHub issue → DoD extraction, `/create-issue` | — | no |
| S2 | Brainstorm → design doc | `/brainstorm` → `superpowers:brainstorming` → `brainstorming-extension` reroutes to design gate | `brainstorming`: one question per message, 2–3 approaches, section-by-section approval, spec to `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`, inline self-review, user file-review gate | no |
| S3 | Design review gate | `/review-design`: 5 (or 6) reviewer personas → `APPROVED\|NEEDS_REVISION`, ALL must approve, max 3 iterations → Override/Defer/Revise/Cancel | — | no |
| S4 | Plan / decompose | Architect agent → `PLAN.md` with 6 required sections; WU ≤ ~5 files; explicit deps; integration WUs | `writing-plans`: mandatory header, `### Task N` blocks with `Files:`/`Interfaces:`/5 checkbox steps; banned placeholder tokens; inline self-review | no |
| S5 | Plan review gate | 3 fresh reviewers (Feasibility, Completeness, Scope&Alignment) `PASS\|FAIL`, any BLOCKING → FAIL, max 3 → Override/Revise/Simplify/Cancel | (legacy plan-reviewer prompt) | no |
| S6 | Work-unit tracking | Beads: `bd create --type epic/task --parent …`, `bd dep add`, `bd ready`, labels, `bd close --reason` | `.superpowers/sdd/progress.md` ledger | Beads: yes (external CLI) |
| S7 | Execution method | human picks | `writing-plans` handoff question | no |
| S8 | Worktree isolation | `~/Developer/<project>-worktrees/<agent>`; layered merge pipeline | `using-git-worktrees`: detect isolation, native tool first, `.worktrees/<branch>` fallback, must be git-ignored, baseline green; never on `main` | no |
| S9 | Implement one WU | Coder subagent (or codex/gemini adapter) with spec, numbered DoD, file-scope allow-list, TDD, no self-certify | SDD implementer: brief via `task-brief`, TDD RED/GREEN evidence, status `DONE\|DONE_WITH_CONCERNS\|BLOCKED\|NEEDS_CONTEXT`, explicit `model:`, never parallel | no |
| S10 | Validate | orchestrator runs tsc, eslint, tests, coverage cmd from `.coverage-thresholds.json` (blocks task completion + PR creation), file-scope via `git diff --name-only`; fail → IMPLEMENT (max 3) | `verification-before-completion`: proving command run *in the same turn* | shell, prompt-triggered |
| S11 | Per-WU adversarial review | ALWAYS a fresh subagent, sees only spec/DoD/diff, `PASS\|FAIL`; fail → fix → re-validate → FRESH re-review (max 3) → ESCALATE | one reviewer, two verdicts; `review-package BASE HEAD`; Critical/Important → fix → re-review; ⚠️ resolved by controller | no |
| S12 | Commit | `git add <file-scope>`, `feat(wu-N): … Reviewed-by: adversarial-review (PASS)`, `bd close`, checkpoint report, update project-context + `SERVICE-INVENTORY.md` | commit per task; ledger line | no |
| S13 | Debugging discipline | — | `systematic-debugging`: 4 phases, one hypothesis, failing test before fix, hard stop at 3 | no |
| S14 | Final whole-branch review | cross-unit checks → `Ready for PR: YES/NO` | `code-reviewer.md` over `merge-base..HEAD`, most capable model; one omnibus fix | no |
| S15 | Pre-PR knowledge capture | mandatory `/self-reflect` before PR; KB edits ride in the same PR | — | no |
| S16 | PR creation | `gh pr create` with body template; coverage gate blocks creation | `finishing-a-development-branch`: tests green → 4-option menu; typed `discard` | no |
| S17 | PR shepherd | `MONITORING → FIXING → HANDLING_REVIEWS → WAITING_FOR_USER → DONE`; `gtg` JSON primary signal; 60 s poll; 4 h soft timeout; DONE only when merged | — | gtg: yes |
| S18 | Handle PR comments | filter, triage by CodeRabbit severity markers, fix / create issue / respond, reply to EVERY thread with attribution, resolve via GraphQL, post-push new-comment check loop | `receiving-code-review`: verify before implementing, no sycophancy, clarify-all-before-any, one fix at a time | two shell scripts |
| S19 | Merge | human approval (`waiting:human`) → `gh pr merge --squash --delete-branch`; rebase on conflict | finish menu | no |
| S20 | Release / deploy | Release Engineer: 7 gates + rollback rubric | — | no |
| S21 | Close | `bd close <epic>`, `gh issue close`, "Landing the plane" | — | no |
| S22 | Learning / curation | `/self-reflect`, Knowledge Curator: CodeRabbit "Learnt from" extraction, ACCEPT/REJECT/TRANSFORM, canonicalize, dedupe | — | fetch scripts only |
| S23 | Handoff | `/handoff`: 10-section doc + one-sentence resume instruction | — | no |
| S24 | Status / health | `/status` (11 checks), `/external-tools-health`, `bin/estimate-cost.sh` | — | partly |
| S25 | Cross-model execution | `external-tools` adapters (`health\|implement\|review`), JSON envelope, error taxonomy (`auth_*` always escalate), scope verification, budget breakers | — | yes (shell) |
| S26 | Visual review | Playwright screenshots of the built UI | — | no |
| S27 | Parallelism | fan-out IMPLEMENT for independent WUs | `dispatching-parallel-agents`: disjoint file scopes only | no |

---

## 3. What metareview already brings that the reference lacks

| Asset | Where | Why it matters for lights-out |
|---|---|---|
| Hash-chained, replayable FSM run store with fork lineage and per-lineage escalation | `internal/fsm/{machine,run,record}` | the durable memory an unattended driver resumes from |
| A real adjudicating judge (`match-then-adjudicate`, threshold 0.7; `unverified_no_evidence` kept rather than dropped) | `internal/fsm/kind`, `internal/fsm/judge` | every review verdict is machine-adjudicated; multi-provider |
| Differential proofs `pin`/`reproduction`/`deletion` verified by mutation (`prove`, `pins_proven`) | `internal/fsm/kind/prove*.go`, `internal/mutation`, `internal/testconv` | fixes are *proven*; the same `Reproducer` machinery verifies TDD in the build unit (§5.8) |
| Re-review-after-fix by graph (`sdlc-loop-clean`) | `workflows/sdlc-loop-clean.yaml` | carried into every fix path as `fix-loop-proved-clean` (§6) |
| Consent-hashed commands, minimal child env, audited `cmd_call` | `internal/fsm/cmdexec` | the safe way to fork `claude -p` / `codex exec` / `pi` / `gh` / `bd` |
| Convergence DSL | `internal/fsm/converge` | per-run circuit breakers; routed in family loops (§5.3) |
| Evidence receipts | `internal/evidence` | validation is a typed receipt |
| Sharded review | `internal/shardpack` | large epics are reviewable |
| Nine artifact lenses; epic-ready's four integration lenses | `rubrics/*`, `internal/lens` | richer than metaswarm's gates |
| Require-lenses gate + review-evidence marker over exact `base..HEAD` | `internal/reviewers/adversarial.go` | a gate cannot PASS on structure alone |
| Overrides with requester ≠ granter | `internal/findings/override.go` | the privileged disposition of an escalated task's findings (§5.3) |
| Git-native pre-push gate (fail-closed) | `hooks/git`, `internal/status` | enforcement outside any model |
| Post-merge learning with a discard rubric | `internal/learning` | S22 exists |
| Mutation-report ingestion (Stryker + gremlins) | `internal/mutation` | consumed by the `mutate` node from Phase 1 |

---

## 4. Gap analysis

Legend — **HAVE** · **PARTIAL** · **MISSING** · **DON'T** (§11). "Phase" = §9. "Gates" = Superpowers
invariant ids (G-*) or metaswarm rules this stage makes machine-checkable.

### 4.1 Lifecycle stages

| Stage | Reference capability | metareview today | Class | Phase | Gates made deterministic |
|---|---|---|---|---|---|
| S0 prime | inject repo knowledge + recovery state | `context build`; knowledge never reaches node prompts | PARTIAL | 5b | — |
| S1 entry / triage | recovery check, Simple/Complex, issue → DoD | none (`tasksource` drops arrays, labels, parent, deps) | MISSING | 3 | Simple path = one-task decomposition; size class from policy |
| S2 brainstorm / design | Socratic design; hard gate before code | nothing authors | MISSING | 3 | G-BS-1/5/6/7/8 |
| S3 design review gate | 5–6 personas, ALL approve, 3 iterations | `review artifact` with 9 lenses — stronger, but scaffold-only (#2 §E) | PARTIAL | 3 | 3-iteration cap → `ESC_DESIGN_ITERATIONS` |
| S4 decompose / plan | plan with WUs, deps, file scope, interfaces, checkpoints | none | MISSING | 3 | G-WP-1..9; dep graph acyclic |
| S5 plan review gate | 3 reviewers, 3 iterations | `review artifact` (no plan rubric; no dep validation) | PARTIAL | 3 | any BLOCKING → FAIL; `ESC_PLAN_ITERATIONS` |
| S6 WU tracking | Beads read+write; ledger | reads `.beads/issues.jsonl` only; never runs `bd` | PARTIAL | 1 (write-through) / 3 (`bd create`) | one task `in_progress`; close-with-reason on PASS |
| S7 execution method | human choice | n/a | — | 1 | PD: `task-build-loop` |
| S8 worktree isolation | per-task worktree, ignored, baseline green, never on `main` | fork asks the human to `git worktree add`; concurrent loops need isolation (`CLAUDE.md` Lifecycle Placement) | PARTIAL | 0b | G-WT-1..7; `on_default_branch` |
| S9 implement | brief → TDD → commit → status | `agent-edit` fixes *known bugs*; no implement-from-brief | MISSING | 1 | G-TDD-1/2/3 engine-verified (§5.8); G-SDD-3/4/5/6 |
| S10 validate | tsc/lint/tests/coverage by the orchestrator | `evidence run`; task-done needs no coverage/lint receipts | PARTIAL | 1 | engine-minted receipts bound to a commit; `coverage_ok`, `lint_ok`, `typecheck_ok` |
| S11 per-WU adversarial review | fresh reviewer, 3 retries | **HAVE and stronger** (recheck only in `-clean`) | HAVE | 1 (`fix-loop-proved-clean`) | — |
| S12 commit + close WU | conventional commit, `bd close`, inventory update | `commit_exists`; no `bd close`; no inventory writer | PARTIAL | 1–2 | `Metareview-Run:` trailer; `bd close` on PASS |
| S13 debugging discipline | 4 phases, one hypothesis, failing test before fix, 3-attempt stop | `reproduction` proof + `pins_proven` encode "failing test before fix"; per-lineage escalation encodes the 3-attempt stop; the four-phase root-cause method is prompt material (§4.2) | HAVE (fix loop); build failures via `tdd_verified` | 1 | G-DBG-3/4 |
| S14 final whole-branch review | cross-unit consistency | **HAVE**: `epic-review-loop` + `review pr-ready` | HAVE | 2 (`finalize-loop`) | — |
| S15 pre-PR knowledge capture | `/self-reflect` before PR; KB edits ride in the PR | `learn --post-merge` only | PARTIAL | 5b | `curate` → `curate-commit` before `finalize` |
| S16 PR creation | `gh pr create`, coverage gate | none | MISSING | 4 | G-FIN-1; pr-ready PASS; `coverage_ok` |
| S17 PR shepherd | monitor gtg/CI/threads, fix, escalate, 4 h timeout | none | MISSING | 4 | typed states; `ESC_SHEPHERD_TIMEOUT`, `ESC_CI_FOREIGN`, `ESC_MERGE_CONFLICT` |
| S18 PR comments | fetch threads, triage, fix, reply, resolve, re-check | flat `gh pr view`; substring blocking (#2 §F) | PARTIAL | 4 | per-comment trust (§5.12); every trusted thread replied |
| S19 merge | squash-merge policy | none | MISSING | 5a | external-condition merge (§5.12.5) |
| S20 release / deploy | 7 gates + rollback | `runtime` receipt kind constant only (#2 §G) | MISSING | 5a (declared-cmd driven, optional) | runtime receipts; `runtime_verified` |
| S21 close | `bd close`, `gh issue close`, land the plane | none | MISSING | 5a (Beads close rides the PR; issue close after merge) | — |
| S22 learning + curation | extract, canonicalize, dedupe, prime back | `learn --post-merge` (append-only; own rows never re-read in non-Beads repos) | PARTIAL | 5b | curation with `supersedes`; repo-scoped sessions; post-merge `learn` node |
| S23 handoff | 10-section doc + resume sentence | none | MISSING | 5b | `factory export` + `factory resume` |
| S24 status / health | 11 checks, tool health, cost | `status`, `setup --check`; no per-job status | PARTIAL | 0a (health) / 6 (cost) | — |
| S25 cross-model exec | codex/gemini adapters | judge routes by model name; no process adapters | PARTIAL | 0a | `runner:` adapters; class routing; error taxonomy |
| S26 visual review | Playwright screenshots of the built UI (distinct from §14's design-phase prototyping) | none | MISSING | 7b (optional) | — |
| S27 parallel WUs | fan-out by disjoint file scope | `fix-fanout` specced not built | PARTIAL | 7a | disjoint-scope check; per-task clones; `integrate-branch` |

### 4.2 Small capabilities

| Capability | Reference | metareview | Class | Phase |
|---|---|---|---|---|
| Session-start self-heal | `session-start.sh` Phase 3.5 | `setup --install-hooks` verifies only | PARTIAL | 7b |
| `bd prime` dedupe with the beads plugin | `session-start.sh` | n/a | DON'T | — |
| `/create-issue` | `skills/create-issue` | none | MISSING | 3 (`file-issue`) |
| `/prime --work-type` | `commands/prime.md` | `context build` | PARTIAL | 5b |
| `/handoff` | `skills/handoff` | none | MISSING | 5b |
| `/status` 11 checks; tool health; cost | metaswarm | `status`, `setup --check`; tokens audited, no $ | PARTIAL | 0a / 6 |
| `/migrate`, `/update` | metaswarm packaging | n/a | DON'T (§13 covers metareview's own compatibility) | — |
| `todo-management` (Beads only, one `in_progress`) | template | n/a | absorbed by S6 | 1 |
| `task-completion-checklist`, `build-validation`, `coding-standards`, `testing-patterns`, Superpowers `systematic-debugging` (4 phases, one hypothesis) | guides / skill | rubrics cover part | prompt material for `implement` (incl. the `implement_rejected` retry brief) and reviewer nodes | 1 |
| `SERVICE-INVENTORY.md` maintenance (metaswarm: repo root; metareview reads both `SERVICE-INVENTORY.md` and `docs/SERVICE_INVENTORY.md`; the writer targets whichever exists, else `docs/SERVICE_INVENTORY.md`) | orchestrator writes after each WU | epic-ready checks it | PARTIAL | 2 |
| `UI-FLOWS.md`, UX reviewer | design gate | none | DON'T | — |
| External-tool `health` + error taxonomy | `_common.sh` | judge retry ladder only | PARTIAL | 0a |
| Coverage-threshold enforcement (blocks task completion + PR creation) | metaswarm | `make cover` gates metareview itself only | PARTIAL | 1 (`coverage_ok`) |
| Cost circuit breakers | `external-tools.yaml` (prompt-enforced) | `budget.tokens` per run | PARTIAL | 6 |
| `finishing-a-development-branch` menu, typed `discard` | skill | none | PD `finish: pr`; `factory cancel` | 0b |
| `dispatching-parallel-agents` | skill | lenses fan out | PARTIAL | 7a |
| `writing-skills`, `using-superpowers` | meta / bootstrap | n/a | DON'T | — |
| Brainstorm visual companion | skill scripts | none | DON'T | — |
| metaswarm ops personas (Metrics, Slack, SRE, Customer-service, Swarm coordinator); Researcher | `agents/*.md` | none | DON'T for v1 (Researcher = a step of `author`) | — |
| Referenced-but-missing metaswarm artifacts | inventory | — | DON'T (never existed) | — |

### 4.3 Engine gaps

| Engine gap | Evidence | Phase |
|---|---|---|
| **Error model:** executor errors and gate misses are terminal; recovery is a fork; `Attempt = len(Lineage)+1`, escalated at 3; convergence stops are terminal; no cancel | `machine.go:678-690, 768-806, 830-857`, `fork.go:32-43` | 0b |
| **No composition** (`ParentRunID/Lineage/ForkedAtSeq` mean *retry fork* to `fork.go`, `record.go`, `fold.go`) | `fork.go:38,221`, `fold.go:441-455` | 0b |
| **Single-loop grammar**; `failed` reserved; one gate name per edge; closed `Outcomes`; `cmd:` only on `cmd` kind; `$VAR` not substituted into `convergence`; convergence validated at parse as typed ints | `workflow.go:222-224, 404, 466-468, 515-553`, `run/types.go:35`, `resolve.go:56-95` | 0b |
| **Per-run constant vars**; map params not scanned; `${…}` needs a per-kind field table | `workflow.go:612-662`, `fork.go:135-162`, `resolve.go:67-83` | 0b |
| **No job→workflow cmds channel**; consent digest keyed on worktree-absolute paths | `machine.go:112-118`, `resolve.go:100-151` | 0a |
| **Exec modes** `inline\|subagent\|fork`; exec that ran not audited (`needs_input` is `EmptyData`) | `workflow.go:125`, `fold.go:358-360` | 0a |
| **Model/effort** via `$VAR` only; no class routing | inventory | 0a |
| **No unattended driver** | `docs/fsm/driving-a-workflow.md` | 0a |
| **Tree-change guard and `FixEntryHead` keyed on the literal kind `agent-edit`** (transition, init and `fix_baseline`); fork-executed nodes never re-read the tree | `machine.go:592-631, 675-695`, `fold.go:227-244, 330` | 0b |
| **Timeout ceiling** 3600 s | `workflow.go:44,298` | 0a |
| **Convergence atoms** lack `wall_clock`; counters read `Iteration`/`Tokens` with no answer baseline | `converge.go:140,201-264,371-382`, `types.go:304` | 0b |
| **Export** `knownKinds` hand list; var values hashed; single 5 MiB in-memory bundle | `export.go:32,369,381-387,427-430` | 0a |
| **`status` treats every non-terminal run as `ABANDONED`** | `status/fsmruns.go:151`, `status.go:166-171` | 0b |
| **Fork executors and the fold cannot see `record` events**; `RecordData` is a no-op in the fold | `fold.go:101`, `machine/types.go:42` | 0b |
| The FSM never writes the marker; `validateFromRunDiff` compares the **init** head | `main.go:718-773` | 1 |
| Receipts keep hashes only (no test report), no revision binding | `receipt.go:33-45` | 1 |
| `agent-edit` output has no status/report channel | `kind.go:678` | 1 |
| **`repo.Root` is one root for diff, committed outputs, transient state, and committed `.metareview/*`**; walks up from a worktree | `root.go:27-45`, `wiring.go:107-125`, `gitpolicy.go:40-46` | 0b |
| **Engine git hardening does not cover `filter.*`, `core.hooksPath`, in-tree `.gitattributes` selecting global drivers; `-c diff.external=` makes any `diff=` attribute fatal** (verified live, §15) | `gate/git.go:59-61` | 0b |
| **Knowledge readers read only `.beads/knowledge/`**; writers pick a path by repo mode | `knowledge.go:55`, `learning/knowledge.go:51-56` | 5b |
| No task scheduler / dependency graph reader | `tasksource`, `epicsource` | 2 |
| GitHub: three read-only `gh` calls | inventory | 4 |
| Surviving mutant → `PASS_ADVISORY`; `runs.jsonl` `coveredPaths` has no writer | inventory | 1 / 7b |
| Go-hardwired test command in the shipped proved loop; zero-config detection unbuilt | `docs/specs/2026-09-01-zero-config-setup.md` | 7a |

---

## 5. Architecture

### 5.1 Principle: gates outside the model, judgment inside, humans at typed escalations

Unchanged from `docs/ARCHITECTURE.md` §1 and extended: **every invariant observable from git, files,
engine-minted receipts, or the run store is a gate atom.** The model does judgment inside a node whose
*output contract* the engine validates and, wherever the output is a claim about the tree, **re-derives
itself** (§5.8, §5.12). A human is consulted only at a typed escalation that parks the run in place
(§5.3); there are no conversational pauses.

Superpowers' measured lessons are adopted as constraints on node prompts: one reviewer/two verdicts;
reviewer and judge classes may carry a tier floor in the routing profile (data, §5.4); positive dispatch
*recipes* beat prohibition lists; literal tripwires ("do not flag", "at most Minor", "the plan chose") are
refused by a **prompt validator** in the `review-lenses` dispatch path (Phase 1); exact values live only in
the brief, never restated.

### 5.2 Workflow families are namespaces, not fold state

The bug loop's fold (`Findings → Confirmed → Status → Pins → Commit`) is the only cross-iteration
accumulation any family needs; every other family's state is "the latest output of node X", persisted in
`Snapshot.NodeOutputs[node@iter]`, plus the driver decision records the fold indexes (§5.3). A `family:` key
selects (a) the **gate-atom table** (shared + family) and (b) validation of which kinds may appear; **all
existing kinds are shared** (`prove`/`still-present` are bug-family-only because they read
`Confirmed`/`FixEntryHead`). Nothing is added to `run.Delta` for family state; the additive `Snapshot` fields
are listed in §5.3/§13 (all `omitempty`, empty when folding a pre-revision log). `family:` absent = `bug`.
**Shipped YAMLs:** `sdlc-loop`, `sdlc-loop-clean`, `sdlc-loop-proved`, `review-loop` are byte-unchanged;
`epic-review-loop` gains one optional var (`CONTEXT`, §5.3 — a hash change, stated in §13).

| Family | Kinds it adds |
|---|---|
| `bug` (existing) | — |
| `build` | `implement`, `mutate`, `task-close` |
| `taskset` | `taskset-load`, `next-ready-task`, `inventory-update`, `integrate-branch` (7a) |
| `artifact` | `author` |
| `shepherd` | `open-pr`, `pr-observe`, `branch-observe`, `comment-triage`, `pr-reply`, `pr-resolve`, `push`, `merge-check`, `merge`, `merge-from-base`, `resolve-conflicts`, `runtime-check`, `rollback`, `close-issue`, `task-close` (the post-rollback `reopen`), `beads-commit`, `learn` |
| `factory` | `intake`, `curate`, `curate-commit`, `task-close`, `beads-commit` |
| shared | `child-workflow`, `await`, `record-marker`, `marker-check`, `gate-review`, `sync-branch`, `verify-tdd` (`mode: task` in `build`, `mode: suite` — written `validate` in §6 — anywhere), `author` (`resplit` uses it in `taskset`), `file-issue` |

Trade stated: `converge.Payload` strips `NodeOutputs`, so a `cmd` convergence atom cannot see family
state; family loops use `max_iterations`/`budget`/`wall_clock`.

### 5.3 Engine changes (Phase 0a/0b)

**Grammar (non-bug families; shipped bug workflows unchanged).**
- Any number of `loop: true` edges; every cycle must contain at least one; each traversal increments
  `Iteration` and resets `NodeAttempts` for the new iteration.
- **Routed convergence with answer-re-armed windows.** Every convergence counter — `max_iterations`,
  `budget.tokens`, `wall_clock` — is measured **since the latest answer**: the fold keeps
  `AnswerWindow{iter, tokens, at}` (set by every `await` output and `park-answer`; empty = run start).
  Convergence is evaluated at a **loop-carrying non-await state** (the source of a `loop: true` edge)
  only when the edge that would otherwise be taken is a loop edge; the stop sets the atom `converged`
  (`converged:<class>` for `overflow | stalled | custom | wall_clock | budget`). **Edge precedence:**
  terminal-outcome edges → forward (non-loop) edges → the `converged` edge → loop edges. Because the stop
  is evaluated only at the state whose loop edge would be taken, the parse rule is per **loop source**,
  not per cycle: **every non-`await` source of a `loop: true` edge carries a `converged`-gated edge**
  (`loop_source_without_converged`); a `converged` self-edge is refused; `await` sources are exempt (an
  answer supersedes convergence and re-arms the window), so a cycle whose loop edges are all sourced at
  awaits is accepted. Every cycle still contains at least one loop edge (`cycle_without_loop`).
- Edges remain `{from, to, gate, outcome, loop}` with **one gate name**; conjunctions are named composite
  atoms (§7). A shared atom `node_output_present` gates unconditional edges. An edge out of an `await` may
  name the pseudo-target **`@from`** = the state that entered the await (recorded in the await's context) —
  the `retry` convention.
- **Explicit `failed` edges** are allowed in non-bug families with `outcome: failed`: an ordinary
  transition (exit 1, no `GATE_FAILED`), flagged `explicit_fail: true`. `cancelled` is a machine-reserved
  terminal like `failed` (never declared; `chose:cancel` edges target it with `outcome: cancelled`).
- Outcomes: `run.Outcomes` gains `built`, `approved`, `released`, `split`, `cancelled`. Classes: **pass** =
  `fixed clean built approved released`; **neutral** = `split cancelled` (ignored by `Escalated()`);
  **non-pass** = `reviewed stalled overflow custom failed` (`reviewed` keeps today's class). `runs.jsonl`
  verdict/status: pass → `PASS`/`passed`; `split` → `SPLIT`/`split`; `cancelled` → `CANCELLED`/`cancelled`;
  non-pass → `NEEDS_REVISION`/`ESCALATED` as today.
- Params: kinds declare `KindInfo.OutputFields` (static field table), `cmd_params` (`test_cmd`,
  `mutate_cmd`, `health_cmd`, `rollback_cmd`), `MutatesTree`, `FixEntry` (`agent-edit`, `implement`), and
  **tree affinity** (`clone | driver | throwaway`, §5.9).
- Parse rules that make every reachable outcome routed: **`child_outcome_unrouted`** — a `child-workflow`
  state routes every outcome class its child (by name, from the registry) **can produce** = the child's
  declared `done[…]`/`failed[…]` outcomes ∪ the classes of the child's *declared* `convergence:` atoms
  (`max_iterations`/`budget`/`wall_clock` → overflow, `no_*_progress` → stalled, `cmd`/`not` → custom)
  ∪ {`escalated`, `cancelled`}, using the map pass→`child_pass` (a pass-class outcome may instead be routed by its
  specific atom, e.g. `child_built`), `reviewed`→`child_reviewed`, `split`→`child_split`, explicit
  `failed`/`stalled`/`custom`→`child_failed`, plus always `child_escalated` and `child_cancelled`.
  **Machine-level failures need no edge:** a child that ends `failed` through `GATE_FAILED`, an executor
  error, or a deterministic failure raises `child_failed`; when the state has no `child_failed` edge the
  driver parks the parent on **`child-park {state, child_run_id, error}`** (`ESC_CHILD_FAILED`, choices
  `retry` = restore + re-drive from the child's checkpoint, `cancel`), so no infrastructure failure ends
  a job silently. **`runner_exhaustion_unrouted`** — a node with a runner (or a class whose profile binds
  one) routes `node_exhausted`, except in a workflow with no `await` states (bug and `artifact-review`
  families), where exhaustion is a driver `tool-park` on the parent; **`await_choice_unrouted`** — every
  declared choice has a `chose:` edge.
- `$VAR` substitution extends to `convergence` scalars (accepted at parse, type-checked after `Resolve`) and
  `await.code`.

**`await` — a non-terminal park.** Host-executed. Params `code` (registered `ESC_*` per family; a data
registry; may be `$VAR`), `choices`, `context` (an interpolated map; `${@last_process_error}` is engine
implicit). Instructions carry `{code, choices, context, from}`; output `{choice, by, note?, context}`, where
`context` is **attached by the machine at record time** from the persisted `needs_input` envelope (a
`--data` that supplies its own `context` is refused), so `${<await>.context.<k>}` is never
human-typed. The `{…}` payload names in §5.6 are documentary; the declared `context:` maps in §6 are the
contract. In lights-out mode the driver records nothing and exits `PARKED{run_id, state, code, choices,
esc_id}` (exit 3). Answering is `fsm record node-output --node <state> --data {choice…}` (wrapped by
`factory answer <esc-id> --choice <name> [--note] [--signed <file>]`); `note` is mandatory for
`override`. `defer` is a driver verb (record
nothing). A park is not a failure: no lineage attempt, not terminal, `Escalated()` ignores it. `esc_id =
<run_id>:<state>:<iter>`. `status` reports a run whose current node is an `await` with no output — or whose
latest driver park has no answer — as **parked** (outside `MustClear`). An answer's `note` reaches later
nodes as `${<await-state>.note}`.

**Driver-level parks.** Budget trips, consent changes, runner exhaustion inside an await-less child, an
unrouted child failure, and a protected-path refusal at import are `record` rows on the top run —
`budget-park {kind}` / `consent-park {workflow, old, new}` / `tool-park {child, state, error}` /
`child-park {state, child_run_id, error}` / `import-park {state, paths[], refused_head, output_sha256}`
(`ESC_PROTECTED_PATHS`; the refused `FETCH_HEAD` is parked under `refs/metareview/discarded/…` and the
adapter's pending output kept in the driver's spool, so `allow`† re-runs the import of *that* head with
the allowance and then records the output — agent work is kept, not redone; `cancel`) with `esc_id =
<run_id>:driver:<seq>` — answered by a
`park-answer {esc_id, choice, by, signature?}` row (single-use), never via `node-output`. Job-record append verbs: `factory job
add-credential`, `factory job set-budget` (`budget-raised`), `factory job accept-consent`
(`consent-accepted`, privileged); the consented set = creation set ∪ accepted rows, carried in
`InitData.Consent` (hashed) so `DecodeIn` reads it from the run.

**Reserved record names.** Records the engine or scheduler interprets — `child-spawn`, `budget-park`,
`consent-park`, `tool-park`, `child-park`, `import-park`, `park-answer`, `answer-relay`, `task-skipped`,
`task-overridden`, `disposition`, `tree-publish` — are driver-reserved: `fsm record` refuses them without `--driver` (a
pre-revision log can only contain them if a user typed them; the fold treats such rows as data, and the
Phase 0b golden replays include a fixture with a colliding user record). The fold indexes them into
`Snapshot.Records[name][]` (omitempty) so fork executors and atoms can read them.

**Cancel.** `fsm cancel --run <id> [--reason]`: a machine-level transition from any non-terminal state to
`cancelled`, taken after any in-flight subprocess exits or is killed; neutral.

**Retryable errors.** Adapter failures `rate_limited | timeout | network_error | context_too_large |
tool_crash | empty` and the sync failure `import_diverged` map to `ERR_PROCESS_<TYPE>` and are
*retryable*: the machine appends a `warn` event `{code, state, iter, attempt}`, leaves the node output
absent, and for `MutatesTree` nodes restores the node's tree to its entry head (§5.9 restore protocol; the
discarded head is first parked under `refs/metareview/discarded/<run>/<seq>`). The fold derives
`NodeAttempts[state@iter]` and `LastProcessError[state@iter]` **only from `warn` events whose code is in
the retryable set**. The routing ladder (§5.4) may change model/runner between attempts, recorded as an
`mrv_exec_override` record and applied at dispatch. When `NodeAttempts ≥ max_node_attempts` (node key,
default 3) the machine **does not re-execute** the node; transitions are evaluated with the output absent:
`node_exhausted` is true and carries `LastProcessError`; every other atom over an absent output is false.
Non-retryable failures (`auth_expired | auth_missing | tool_not_installed`) set `node_exhausted`
immediately. Judge/`cmd`/`prove` executor errors keep today's terminal semantics. `ERR_CHILD_TORN` is
**not** a retry: the driver runs `advance --repair` on the child and resumes it.

**Fork-executed tree-mutating nodes.** After a `fork` execution of a `MutatesTree` kind, the machine
re-reads head/tree (of the driver worktree, after the §5.9 import), appends a `tree` event, and uses the
fresh head for the transition. `FixEntryHead` is set on entry to a `FixEntry` kind (additive
`TransitionData.ToFixEntry`, `InitData.InitialFixEntry`; `fix_baseline` accepts any `FixEntry` kind; the
legacy `agent-edit` name rule applies when the fields are absent).

**Composition is driver-level.** A child run is an ordinary top-level run. `child-workflow` is
host-executed with params `workflow`, `base` (default `$JOB_BASE`), `vars` (map), `goldens?`. **Goldens
grammar:** a list of sources, each `<state>.<field>` | `@from.<field>` | `$VAR` (a JSON list); a source
whose state has no output, whose kind lacks the field, or whose var is empty contributes `[]` (never an
error). Element shapes accepted: `Bug`, `Finding`, and any `{file, line, desc, …}` record (`PrState.
failing_findings`, `comment-triage.findings`, `verify-tdd.failures`, `gate-review.findings`). The driver
**concatenates** the sources in order (deduplicated on `<file>:<line> <desc>` — first occurrence wins,
a later duplicate's non-empty `SourceID` is copied onto it, and segment 0 (`$GOLDENS`, when first) is
never collapsed so its indices are stable; `golden_sources` offsets are post-dedup), converts each
element to `run.Golden{Comment: "<file>:<line> <desc>", SourceID: <Bug.ID | "">}` (a `Finding` has no
id today; a bug-derived finding carries its bug's; a golden from `comment-triage.findings` keeps its
`thread_id`), places the dispositioned-bugs source **second** (right after segment 0), truncates to
`run.MaxGoldens` (64, a fold-enforced replay constant — kept, not raised) **from the tail** so neither
the caller's `$GOLDENS` segment nor the dispositions are ever cut (a truncation is recorded in the
`child-spawn` row and surfaces as a warning; a `$GOLDENS` list alone over 64 is refused at spawn with
`ERR_GOLDENS_OVERFLOW` — `comment-triage` emits at most 32 findings per cycle and `pr-observe` at most
16 `failing_findings`, the rest wait for the next cycle), records
`golden_sources[{source, start, len}]` in the `child-spawn` row, and writes the goldens file for `init
--goldens`. `Golden.SourceID` is additive: a matched golden's bug keeps the identity of its source, so a
bug re-supplied as a golden across runs is *the same bug*, not a paraphrase. The driver also appends, as the
second source, the bugs of every *covering* `disposition` record in scope (§ Dispositions) — the
re-review after a `waive`/`dismiss`/`override` re-matches them instead of re-deriving ids from prose. The host (1) records
`child-spawn {state, iter, child_run_id, workflow, workflow_hash, base, work_dir, golden_sources[]}`, (2)
inits the child with `--invoked-by <parent_run>:<state>@<iter>`
(`InitData.InvokedBy`, copied by `Fork`; `ParentRunID/Lineage/ForkedAtSeq` stay retry-fork fields),
(3) drives the child to terminal or park, applying the **child retry rule**: a child that ends `failed |
stalled | custom` (not `overflow`, not an `explicit_fail` edge, and not a *deterministic* failure — one with
no `llm_call`/`process_call` since its checkpoint) at `Attempt < MaxAttempts` is forked by the driver at
its checkpoint — the last review state entered, else the last `FixEntry` entry, else the initial state —
after restoring the tree to that head (§5.9), and driven again; (4) records the parent node output
`{child_run_id (lineage root), leaf_run_id, chain_head, seq, outcome, escalated, mock, tokens, path?,
confirmed: [Bug], unverified: [Bug], plan_mandated: [Bug], advisory: [Finding], fixed_golden_idx: [int],
golden_sources: [], goldens: [Golden], note?}` where `escalated = Escalated(leaf) ∨ leaf.outcome == overflow`, `goldens` is the concatenated list the child received (so a consumer pairs indices with the list from the same output), `path` is the
leaf's `author.path` when the child is an `artifact-loop`, `fixed_golden_idx` = the `GoldenIdx` (an index
into the concatenated goldens list, mapped to its source through `golden_sources`) of every confirmed bug
whose `Status` is fixed in the leaf (so a caller that supplied goldens can map fixes back to their source
— thread ids, CI findings — across every fix iteration), and `note` is the leaf's last await note.
**Non-bug children declare their channels.** A bug-family leaf derives `confirmed/unverified/
plan_mandated/advisory/fixed_golden_idx` from its fold. A non-bug workflow (whose fold has no bugs)
declares an **`outputs:`** map in its YAML, each field a `${<state>.<field>}` source over its own states
(e.g. `finalize-loop: {confirmed: ${fix.confirmed:-[]}, fixed_golden_idx: ${fix.fixed_golden_idx@all:-[]},
golden_sources: ${fix.golden_sources:-[]}}`, `epic-build-loop: {advisory: ${schedule.advisory:-[]}}`);
undeclared fields are `[]`. **Verification** happens at record time through a kind seam `DecodeIn(snap,
store, raw)` (read-only run store; also used by `Fork`, which accepts `InvokedBy.run ∈ {RunID} ∪
Lineage`): the child named by the latest `child-spawn` for `state@iter` must have `InvokedBy` matching,
init `workflow/workflow_hash` ∈ `InitData.Consent` and equal to the node's `workflow`, init `base` = the
node's resolved `base`, init `work_dir` = the parent's `WorkDir`, init `mock` = parent's; the leaf's fold
(bug family) or its `outputs:` map resolved against the leaf's own verified `NodeOutputs` (non-bug) must
reproduce every other field (`unverified` = confirmed with verdict ∈ {`unverified_no_evidence`,
`checked_but_unverified`}; `plan_mandated` = confirmed with label `plan-mandated`; `advisory` = rejected
with 0.4 ≤ confidence < 0.7). Mismatch → `ERR_CHILD_MISMATCH` refused at record time. A parked child makes the driver return the child's `PARKED` envelope; the parent
stays at its node. Runner exhaustion inside a bug-family child (which has no `ask-tool`) is a driver
`tool-park` on the parent. `fsm export` lists children with hashes and exports them only with `--deep`.
`VerifyOrigin` gains the child→parent chain-head check.

**Escalated findings become findings.** When a child escalates, the driver projects the leaf's unfixed
confirmed bugs into `findings.jsonl` (status `open`, target = the task, `gitHead` = leaf head). The
`override`† choice of `ESC_TASK_ESCALATED` is a signed answer (note = grant reason) that also records
`override grant` rows on them, records `task-overridden {ids}` (the scheduler treats the task as done), and
**writes no marker** at the task level. `split` dispositions them `deferred` (`deferredTo` = the new task
ids); `skip_*` dispositions them `deferred` to the filed issue; and on `retry_narrowed` the projected
rows are re-supplied as goldens to the re-built task's review child, so `record-marker --scope
task-done` closes those that were matched and fixed (`fixedInRunId` = the leaf) — otherwise they would
stay `open` and fail the epic gate's "unresolved child blockers" check forever.

**Dispositions persist.** `waive`/`dismiss`/policy-`advisory`/**override-grant** outcomes are recorded as
`disposition {ids, bugs[], kind, by}` rows carrying the `Bug` objects, not only ids — `bugs[]` is the
answering await's echoed `context.bugs ∪ context.findings`, so the row names exactly what the human saw
(an override grant is disposition kind `override`; the policy-`advisory` row is written **by the
machine** on the transition that takes `unverified_clear` under `Policy.unverified == advisory`, since
`unverified-check` is node-less). Covering dispositions also reach the bug fold (below), so a
fix-capable child that re-confirms an overridden bug neither fixes nor loops on it. **Identity join, one rule:** a confirmed bug is *dispositioned* iff its `ID` ∈ any
`disposition.ids` in scope, or its `GoldenIdx` names a golden whose `SourceID` ∈ any `disposition.ids` —
which is why the driver re-supplies dispositioned bugs as trailing goldens to every review child (§
Composition; a `@from.context.<k>` whose key is absent is `[]` like a missing field). In scope = every `disposition` row in the **job's run subtree** (an override granted under
`factory.build` must be visible to `factory.finalize`'s `epic-check`, a sibling subtree; standalone runs
use their own subtree). **Two classes of disposition kind.** *Covering* kinds — `waive`, `dismiss`,
`advisory`, `override` — mean "this bug need not be fixed": `plan_conflict`/`unverified_block` exclude
them, `record-marker` accepts them (§5.8), `recheck`'s `findings_empty` and `confirmed_fixable` ignore
them, and a `reviewed` leaf whose confirmed bugs are **all** covered counts as `child_pass`, so a
re-review after an override passes instead of re-finding the overridden bugs. The *directive* kind
`fixable` — recorded by a `fix_code` answer at `ask-plan` or a `treat_as_real` answer at `ask-unverified`
for the bugs it names — means the opposite, "fix this despite its label": it only flips
`confirmed_fixable`/`confirmed_unfixable_only` for a `plan-mandated` or unverified bug and suppresses
the re-park at `plan_conflict`/`unverified_block`; it never satisfies `child_pass`, `record-marker`, or
`findings_empty`, so a `treat_as_real` bug the child cannot fix still blocks. For every await, the row's
`bugs[]` is `context.bugs ∪ context.findings`, and an `override grant` row is written for each element
carrying a `findings.jsonl` `id` (a gate finding included).

**Scheduler decisions are durable.** `task-close`, `task-skipped`, `task-overridden`, and `resplit` also
append a `task-state {id, state: built | skipped | overridden | split, run}` row to the job record;
`next-ready-task` reads the job record, not only its own run's `Records`, so a re-entered or retry-forked
`epic-build-loop` never re-schedules a built, skipped, overridden, or split task (in plan-file mode this
is the only ledger of built-ness; a split task's un-renumbered id is retired by its `split` row, and
`taskset_valid` refuses a retired id in the `depends_on` of the new ids). `task-close` outputs `{id,
prior_status, …}` and refuses to reopen (or re-close) an issue once the job's PR is merged — except the
single sanctioned `reopen: true` call on the `release-loop` post-rollback path (§5.10), which is the one
reopen allowed after the merge.

**Interpolation.** Params and child `vars:`/`base:`/`goldens:` may reference `${<state>.<field>}` where
`<state>` is a state **of the same workflow** and `<field>` ∈ that kind's `OutputFields` (string, int,
list, or path). Resolution is **the latest output at or before the current iteration**
(`NodeOutputs[<state>@i]`, max `i ≤ Iteration`); the resolved `iter` is recorded in the payload; no output
at all → `ERR_INTERP_UNAVAILABLE` unless a default is given (`${triage.replies:-[]}`; defaults may nest;
shell-style, `:-` applies when the output is absent **or** the field is absent/empty).
At a **merge point** (a state entered from several predecessors) the latest-output rule would pick a
stale channel, so a second form exists: **`${@from.<field>}`** resolves against the output of the state
that entered the current state (recorded provenance, the same `from` the await envelope carries) and takes
its default when that state's kind lacks the field **or the state is node-less** (`enter`, `plan-check`).
`<field>` may be a dotted path into a map-valued output field (`${ask-plan.context.findings}` — an
`await` echoes its interpolated `context` map in its output). A third form, **`${<state>.<field>@all}`**,
is the concatenation of a list-valued field over **every** iteration of the run (in order, deduplicated)
— for channels that accumulate across loop re-entries, such as the fixes a `finalize-loop` made in two
`fix` iterations; and **`${<state>.<field>@iter}`** resolves only against an output of the **current**
iteration (default otherwise) — for a channel that must not carry over, such as the indices a
`pr-resolve` maps. `goldens:` sources may use all forms.
Cross-workflow values pass only through child `vars:`/`base:`; list values are JSON-encoded in vars and
passed by `init --vars-file`, never argv. Map-valued params are scanned and substituted recursively.
Per-field grammar at decode: SHAs `^[0-9a-f]{40}$`; ids `^[A-Za-z0-9_=:-]+$` (no leading `-`/`@`); paths
repo-relative, no `..`, no leading `-`; free text never reaches argv.

**Cmds channel.** `init --cmds-from <job cmds file>` merges the job's cmds (by name; the job's entry wins)
before `ResolveCmds`; a workflow lists the names it expects under `cmds_from_job: [test, lint, …]` and may
mark some `optional` (atoms that need an absent optional cmd are false). Runners are **engine-implicit
cmds** `mrv-runner-<name>` with argv `[$METAREVIEW_STATE_DIR/adapters/<name>.sh]`. The **consent digest**
is computed over the *declaration* (name, argv with `$VAR`s unresolved, content hashes of pinned files
relative to the checkout) — worktree-independent; the engine derives the per-worktree resolved pin;
`--allow-custom-cmds` receives the declaration digest. Workflow consent is keyed by canonical name +
hash; an upgrade that changes an embedded workflow's bytes parks in-flight jobs on `consent-park`.

**Engine-implicit vars.** The 21 class vars (§5.4), `RUBRIC`, `MUTATION_POLICY` and `MERGE_POLICY`
(bound from `policy.mutation`/`policy.merge`), **`JOB_BASE`** — a **SHA** re-resolved at every init in the
job to `merge-base(origin/<branch.base>, HEAD)` (never a ref tip, so two-dot ranges stay the job's own
diff after `merge-from-base`; a long-lived parent's folded value is therefore stale after a
`merge-from-base` inside its lifetime — parents use it only as the child default `base`, never as a diff
anchor of their own), `JOB_BASE_REF` (the branch name `branch.base`), `JOB_PR` (the PR number the job
record holds, `""` before `open-pr`), `SHEPHERD_TIMEOUT`, `SHEPHERD_POLL`, `SHEPHERD_OBSERVE_MAX` (from
`Policy.shepherd`; clamped to `max_runner_timeout`), **`ALLOWED_PATHS`** (the union of `paths[]` over the
job record's `allowance {esc_id, paths[], by, at}` rows — the driver appends one for every signed
`allow`, whether answered at an await or an `import-park` — **re-derived from the job record on every
`advance`**, a projection like `Records` rather than a folded var, so `push0`/`validate` re-run after an
`allow` see it and a long-lived `epic-build-loop` sees an allowance granted mid-run; every scope check
reads it, including `mode: suite` validates; a workflow-declared `ALLOWED_PATHS` may only narrow it),
and `CONTEXT` (default `""`; a
`review-lenses` node with `context: $CONTEXT` folds the raw var text — string or JSON list — fenced, into
its input — how a parent passes `FAIL_BEFORE`/advisory findings to a review child). A workflow-declared
same-name var keeps its declaration semantics.

**Marker currency, one rule for every consumer.** A review-evidence marker `{base, headSha}` is
**current at `H`** iff `base` equals the consumer's base, `headSha` is an ancestor of (or equal to) `H`,
and `git diff --name-only <headSha>..H` (driver flags, §5.9) touches only paths in the gate's exclude set
(`docs/metareview/**`, `.metareview/**`) — i.e. nothing reviewable changed since the marker. The exact
match is the trivial case. The rule is implemented once (a git seam on `LatestReviewEvidence`) and used
by `LatestReviewEvidence` (`reviewevidence.go`, today exact on both ends), `validateFromRunDiff`
(`main.go`, today `d.Head != wantHead`), `record-lenses`, `record-marker` (`ERR_MARKER_STALE` = the leaf
head is not current at HEAD), `marker-check`/`markers_current`, and `merge-check`. The pre-push hook is
not a marker reader — it credits review *logs* by commit membership with the same exclude set — so the
logs `gate-review` commits satisfy it directly. The rule is what lets `gate-review` commit its logs
after `mark-epic`/`mark-pr` without staling them. A stale marker is one whose `headSha..H` diff touches
a non-excluded path.

**Three bases, named once.** (1) `JOB_BASE` — the diff anchor (merge-base SHA) every review, marker, and
`verify-tdd` range uses; (2) `refs/metareview/base-tip` — the fetched tip of `branch.base`, the *merge
target* published into the ref source for `merge-from-base`/`resolve-conflicts`; (3) the gate commands'
and pre-push hook's own default (`merge-base(HEAD, main)` of the *local* `main`) — **never used inside a
job**: every in-job `review …` invocation passes `--base $JOB_BASE` (a fresh init's value), and `push`
runs with `METAREVIEW_BASE` set to the `merge-base(origin/<base>, HEAD)` it computes itself for its
protected-path check — never a folded var, because the long-lived shepherd's `$JOB_BASE` predates any
`merge-from-base` inside it and the hook would otherwise see every upstream file as unreviewed. That
fresh merge-base equals the `JOB_BASE` the preceding `finalize-*` init resolved, so the hook gates the
same range the markers attest.

**Sharded review in the FSM.** The `review-lenses` kind gains a shard mode (Phase 2): when the
exclude-filtered diff exceeds the threshold (`contextprofile.DefaultLargeDiffBytes`, 120 KB), the node
cuts it into shards of at most `DefaultMaxBytesPerShard` (60 KB) as the gate commands do, dispatches one
lens pass per shard plus a cross-shard pass, and merges findings with the existing `[shard-xx]` identity
normalisation; the ladder's `context_too_large` step routes to shard mode rather than re-dispatching, and
a job may cap the reviewed diff instead (`policy.review.max_diff_bytes` → `ERR_JOB_DIFF_TOO_LARGE`).
Under a job the gate commands **ignore tree-resident shard results** (`docs/metareview/shards/**` is
agent-reachable content) and rely on this mode.

**Policy in the snapshot.** `init --policy <file>` folds the job's `policy:` block into `InitData.Policy`
(hashed; additive). Policy-reading atoms read `Snapshot.Policy` only.

**Timestamps.** `AnswerWindow.at` is the answer event's `At`; the last event time is derived on demand,
never folded. `wall_clock` = `lastEventAt − max(CreatedAt, AnswerWindow.at)`. The job-level
`budget.wall_clock` counts **driving time** on the driver's `Clock`. Any answer or `park-answer` in a
child makes the driver append an `answer-relay {esc_id}` record (driver-reserved) on **every ancestor**,
which re-arms their `AnswerWindow` too. `Clock` is a seam.

**Envelope.** The `NEEDS_INPUT` builder moves into `internal/fsm/machine`; the payload gains `exec`,
`runner`, `model`, `effort`, `attempt`, `work_dir`, `tool_policy`, `knowledge[]`, `record`, `from`.
`max_runner_timeout` (default 4 h) applies to runner nodes, `mutate_cmd`, and `pr-observe`/`branch-observe`.

### 5.4 Runners and model routing (configurable outside the binary)

**Runners.** Host-executable kinds (`review-lenses`, `agent-edit`, `implement`, `author`,
`comment-triage`, `curate`, `resolve-conflicts`, `inventory-update`) have `AllowedExec = inline | subagent |
fork(runner)`: with `runner: <engine-implicit runner name>` the binary forks the adapter through the
consented cmds path. Interactive hosts keep `inline`/`subagent`. **Under `factory run`, a host-executable
node without `runner:` uses its class's `_RUNNER` binding from the routing profile** — recorded as an
`mrv_exec_override`, consented through the job's profile hash — so the shipped YAMLs run unattended
without changing their bytes. The adapter contract:

| Direction | Content |
|---|---|
| stdin | the `NEEDS_INPUT` envelope (§5.3) — untrusted fields nonce-fenced as judge inputs are; `knowledge[]` and `CONTEXT` are untrusted; the adapter passes `instructions` verbatim and maps `tool_policy` to driver flags |
| stdout | `{"output": <node output>, "tokens": {…}, "error": null}` or `{"output": null, "tokens": {…}, "error": {"type": "<taxonomy>", "detail": "<≤1 KiB>"}}`; empty/unparseable stdout = `empty` (retryable); runner node output cap = `MaxPayload − 1 KiB` |
| exit code | 0 = stdout authoritative; 124 = `timeout`; other non-zero without JSON = `tool_crash` |
| audit | one `cmd_call` (hashes) plus `process_call {runner, model, effort, attempt, duration_ms, tokens}` on the `node_output` event |

Adapters (`claude-p`, `codex`, `pi`, `gemini`) are embedded and materialized by `setup
--install-adapters`; each maps failures to the taxonomy, handles `claude -p`'s empty-output case as
`empty`, and passes the sandbox/permission flags `tool_policy` dictates. **Egress bridge:** inside the
sandbox the adapter launches the embedded `metareview egress-bridge` (TCP `127.0.0.1:<port>` → the
invocation's proxy socket, §5.12.2) and sets the driver's `*_BASE_URL` to it. Adapter fixtures: one fake
driver binary per taxonomy code (`internal/adapterstest`).

**Class routing.** Workflows name classes: `model: $IMPLEMENTER`, `effort: $IMPLEMENTER_EFFORT`, `runner:
$IMPLEMENTER_RUNNER`. Classes `PLANNER`, `IMPLEMENTER`, `REVIEWER`, `JUDGE`, `SYMPTOM`, `SHEPHERD`,
`CURATOR`. Precedence: `--var` → `METAREVIEW_MODEL_<CLASS>` / `_EFFORT_` / `_RUNNER_` env → the selected
`models.yaml` profile → workflow `vars` defaults. Bindings are folded into snapshot vars. `agent-edit` →
`IMPLEMENTER`, `review-lenses` → `REVIEWER`, judge kinds → `JUDGE`, `prove`'s symptom reviewer → `SYMPTOM`.

`models.yaml` (state dir; hashed into the job record):

```yaml
version: 1
tiers: [cheap, mid, capable]
models:
  claude-opus-5:    {tier: capable, provider: anthropic, price: {input_per_m: 15.0, output_per_m: 75.0}}
  claude-sonnet-5:  {tier: mid,     provider: anthropic, price: {input_per_m: 3.0,  output_per_m: 15.0}}
  glm-5.3-flash:    {tier: cheap,   provider: openai-compatible, price: {input_per_m: 0.1, output_per_m: 0.4}}
runners: {claude-p: {}, codex: {}, pi: {}, gemini: {}}      # engine-implicit cmds mrv-runner-<name>
profiles:
  default:
    PLANNER:     {model: claude-opus-5,   effort: high,   runner: claude-p}
    IMPLEMENTER: {model: claude-sonnet-5, effort: medium, runner: claude-p}
    REVIEWER:    {model: claude-opus-5,   effort: low,    runner: claude-p, floor: mid}
    JUDGE:       {model: claude-opus-5,   effort: medium}
    SYMPTOM:     {model: claude-opus-5,   effort: low}
    SHEPHERD:    {model: claude-sonnet-5, effort: low,    runner: claude-p, floor: mid}
    CURATOR:     {model: glm-5.3-flash,   effort: low,    runner: pi}      # decision 12.5; setup asks per class
ladder:
  - {on: [rate_limited, network_error], then: {runner: codex}}
  - {on: [context_too_large],           then: {model: claude-opus-5, effort: high}}
allowed_providers: [anthropic, openai-compatible, codex]
```

`floor` refuses a binding below that tier at job start (`ERR_ROUTING_FLOOR`). `METAREVIEW_MODEL_PROFILE`
selects a profile. `usd` = audited tokens × prices, reconciled against the proxy's per-request accounting.

### 5.5 The unattended driver and the job record

`metareview factory run --job <job.yaml> | --workflow <name> --base <ref> [--resume <job-id>]
[--dry-run]` loops `advance → dispatch → record` until the top run is terminal or parked. `--workflow/
--base` synthesises a minimal job. `--dry-run` is a **driver** flag: GitHub write nodes render their
payload and output `{dry_run: true}`; write atoms require `dry_run == false`.

**`job.yaml`** (schema v1; copied verbatim into the job record at creation and never re-read; the only
mutations are the three audited append verbs of §5.3):

```yaml
version: 1
job_id: <slug>
intake: {intent: "<text>" | issue: <n> | spec_path: <p> | plan_path: <p> | pr_number: <n>}
entry: auto | design | decompose | build | ship | release
size: auto | simple | complex
branch: {template: "factory/{job_id}", base: main}
test_convention: go | typescript | vitest        # python: ERR_JOB_CONVENTION_UNSUPPORTED until Phase 7a
docs: {dir: docs/plans}                          # authored documents: <docs.dir>/<job_id>-<template>.md
policy:
  human_approval: {artifact: none, plan: none}      # keyed by RUBRIC; none | required
  merge: human                                      # auto | human (default human; §12.8)
  finish: pr                                        # pr = remove the job's trees once the top run is terminal | keep
  mutation: required                                # required | advisory | off   (default required)
  coverage: {threshold: 100, metric: statements}    # refused if cmds.coverage absent (ERR_JOB_POLICY_UNBACKED)
  unverified_findings: block                        # block | advisory
  external_review: optional                         # required | optional
  review: {max_diff_bytes: 0}                       # 0 = shard instead (§5.3); else ERR_JOB_DIFF_TOO_LARGE
  curation: {review: auto}                          # auto | human (default auto; §12.12)
  shepherd: {timeout: "4h", poll: "60s", observe_max: "2h"}   # observe_max ≤ max_runner_timeout
  on_exhaust: park                                  # park | cancel  (max_escalations)
  intake: {create_issue: false}
  allowed_warnings: []                              # regex list, annotates test stderr (advisory only)
budget: {tokens: N, usd: N, wall_clock: "8h", max_escalations: N}
cmds:                                               # each: {argv, timeout?, egress?: [host…]}
  test: {argv: [...]}; lint: {...}; typecheck: {...}; coverage: {...}; mutate: {...}
  install: {argv: [...], egress: [registry.npmjs.org]}   # optional; dependency install in the clone (§5.9)
  health: {argv: [...], egress: [api.example.com]}  # optional; rollback: likewise optional
consent: {workflows: {task-build-loop: <hash>, ...}, cmds_sha256: <declaration digest>}
github:
  writes: [open-pr, pr-reply, pr-resolve, push, merge, file-issue, close-issue]
  trusted_authors: [login, ...]
  trusted_bots: [coderabbitai[bot], cursor-bugbot[bot]]
  factory_login: <login>               # never counts as an approver
credentials: {test: [ENV_A], health: [ENV_B]}      # names only; values must be set and ≥ 8 bytes (ERR_JOB_CREDENTIAL_UNUSABLE)
answers: {mode: signed, allowed_signers: <path>}   # signed (default) | agentic (decision 12.7); allowed_signers copied at creation
routing: {profile: default}
```

**Job record** `<state-dir>/jobs/<job-id>/job.jsonl` holds what no run can: the policy copy (hashed), the
top run id, the PR number (written by `open-pr`), the clone/worktree registry, `models.yaml`/adapter/
sandbox-profile hashes, a **copy** of the `allowed_signers` file (verification reads the copy, never the
path), `sandboxed: true|false` and the self-test result, the driver's `{pid, start_time}` (`factory
cancel` signals only a process whose start time matches), and the append-row kinds, each a JSONL row
with a `kind` discriminator: `task-state`, `allowance`, `gate-run`, `review-run`, `rollback`,
`add-credential`, `budget-raised`, `consent-accepted`. `factory run` refuses an existing `job_id` without `--resume` (`ERR_JOB_EXISTS`), and `author`
refuses a pre-existing `DOC_PATH` it did not write in this job. **The run store is authoritative**;
`factory status` is a projection folded from the run subtree; write order: run event first, job row
second; `factory resume` repairs a stale job row.

**Escalations** are the projection "await nodes without an output, or driver parks without an answer"
(`factory escalations`). `override` (the findings ledger) is used exactly for the escalated-task
disposition of §5.3; `waive`/`dismiss` answers are also written to `findings.jsonl` as dispositions
(`waived` / `accepted-risk`, with `dispositionBy/At/Reason`, §13).

**Job-level breakers** (`budget.*`, `max_escalations`) are evaluated by the driver before every `advance`;
tripping one records `budget-park` and stops driving; `factory job set-budget` then `factory answer <esc>
--choice retry` resumes; `on_exhaust: cancel` cancels instead.

**Cancel.** `factory cancel <job-id> [--discard]` (typed confirmation `discard` with `--discard`): `fsm
cancel` on every non-terminal run of the job (top-down; in-flight adapters are killed first), then:

| Resource | keep (default) | `--discard` |
|---|---|---|
| agent clone + driver worktree | left in place, listed | removed after registration check and a clean tracked `.beads/*` check (§5.9) |
| job branch | kept | deleted locally; remote branch never deleted |
| Beads tasks/epic the job set `in_progress` or closed (`task-close` outputs) | reopened, `--notes "factory cancelled <job>"` | same + label `factory:abandoned` |
| open PR | comment "factory job cancelled" | same + `gh pr close` (only if the job opened it) |
| `refsrc/<job>`, `refs/metareview/discarded/<job runs>` | kept until export | removed / pruned |
| run store / job record | kept | kept |

`factory gc` prunes `refsrc/` and discarded refs of exported terminal jobs.

### 5.6 Human touchpoints, classified

**PD** = policy default; **EG** = evidence-gated auto-proceed; **ESC** = an `await` with the listed
choices, each routed in §6. Codes are registered per family (§5.3). Privileged choices (§5.12.6) are marked †.

| Code | Raised by | Choices → effect | Notes |
|---|---|---|---|
| — Simple/Complex | `intake` | PD `size` | `size_simple` is evaluated before `entry_*`, and only for `entry ∈ {design, decompose, build}` |
| — Create issue? | `intake` | PD `intake.create_issue` | |
| `ESC_INTENT_AMBIGUOUS{questions}` | `artifact-loop.author`/`revise`, `epic-build-loop.resplit` (`ask-split`) | `answer` (note → re-author), `cancel` | interactive brainstorming is outside the factory (§11, §12.9) |
| `ESC_SPEC_APPROVAL` / `ESC_PLAN_APPROVAL` (`$APPROVAL_CODE`) | `artifact-loop.approve` when `human_approval.<RUBRIC>: required` | `approve`†, `revise` (note → author), `cancel` | EG otherwise |
| `ESC_DESIGN_ITERATIONS` / `ESC_PLAN_ITERATIONS` (`$ITERATIONS_CODE`) | `artifact-loop` on `converged` | `override`† (→ approve), `revise` (note → revise), `cancel` | `defer` = driver verb |
| `ESC_BASELINE_RED` | `epic-build-loop.load` | `proceed`†, `cancel` | |
| `ESC_CREDENTIALS_REQUIRED{names}` | `epic-build-loop.load` | `provided` (→ load), `skip_tasks` (→ `task-skipped` + issue), `cancel` | names only |
| `ESC_PROTECTED_PATHS{paths}` | `epic-build-loop.load`, `pr-shepherd-loop.push0/push-ci/push`; the import (§5.9) raises it as a driver `import-park` | `allow`† (signed payload names `paths[]`), `cancel` | §5.12.4 |
| `ESC_DEP_DEADLOCK{blocked_ids}` | `epic-build-loop.schedule` | `skip_blocked` (→ `task-skipped` + issue), `cancel` | |
| `ESC_CHECKPOINT` | `epic-build-loop.ask-checkpoint` | `continue`†, `adjust` (note → the next `build`'s `NOTE`), `cancel` | |
| `ESC_TASK_BLOCKED` | `task-build-loop.ask-blocked` | `context` (note → implement), `split` (→ done `split`), `cancel` | |
| `ESC_PLAN_CONFLICT{findings, brief}` | `task-build-loop.ask-plan` (`context:` from `review.plan_mandated`) | `fix_code` (→ review child, goldens = the findings), `amend_plan` (→ done `split`), `waive`† | |
| `ESC_UNVERIFIED_FINDINGS{bugs}` | `task-build-loop.ask-unverified` (policy `block`; `context:` from `review.unverified`) | `dismiss`†, `treat_as_real` (→ review child, goldens = the bugs), `cancel` | |
| `ESC_ACCEPTANCE_UNCOVERED{cells}` | `epic-build-loop.mark-epic`, `finalize-loop.mark-epic` | `waive`†, `cancel` | the coverage matrix (§5.8); an absent matrix mints `PASS_ADVISORY`, never parks |
| `ESC_TASK_ESCALATED{bugs}` | `task-build-loop.ask-escalated`, `epic-build-loop.ask-task`, `finalize-loop.ask-final` (`context:` = `@from.confirmed`) | `retry_narrowed` (note → `@from` via `NOTE`/`CONTEXT`), `split`, `override`† (§5.3), `cancel` | |
| `ESC_INTEGRATION_ESCALATED{bugs, findings}` | `epic-build-loop.ask-integration` (an integration review/fix/validate escalated — `retry_narrowed`/`split` on the last-scheduled *task* would be wrong here) | `retry_narrowed` (note → `fix-integration`), `override`† (§5.3; → `validate`), `cancel` | |
| `ESC_FINALIZE_ESCALATED` | `pr-shepherd-loop.ask-finalize` (a `finalize-loop` child ended `split`/`escalated`) | `retry` (→ `@from`), `cancel` | no PR may exist yet |
| `ESC_PUSH_REJECTED{stderr_sha256}` | `pr-shepherd-loop.ask-push` (the remote refused a push) | `retry` (→ `@from`), `cancel` | |
| `ESC_JOB_ESCALATED{stage}` | `factory.ask-factory` | `retry` (→ the escalating stage), `cancel` | |
| `ESC_TOOL_FAILURE{state, error}` | every runner node's `ask-tool`; driver `tool-park` for await-less children | `retry` (→ `@from`; attempts reset), `cancel` | |
| `ESC_CHILD_FAILED{state, error}` | driver (`child-park`) when a child fails at a state with no `child_failed` edge | `retry` (restore + re-drive from the checkpoint), `cancel` | §5.3 |
| `ESC_BUDGET_EXHAUSTED{kind}` | driver (`budget-park`) | `retry` (after `set-budget`), `cancel` | |
| `ESC_CONSENT_REQUIRED{workflow}` | driver (`consent-park`) | `accept`† (= `accept-consent`), `cancel` | |
| `ESC_CI_FOREIGN` / `ESC_COMMENT_UNTRUSTED{threads}` | `pr-shepherd-loop` | `retry` (→ observe), `handled_externally`†, `cancel` | |
| `ESC_MERGE_CONFLICT{paths}` | `pr-shepherd-loop.ask-conflict` (from `resolve` or `sync`) | `retry` (→ `@from`), `handled_externally`† (→ `sync`), `cancel` | |
| `ESC_PR_CLOSED{closed_by}` | `pr-shepherd-loop.ask-closed` | `retry` (→ open; **privileged unless `closed_by == factory_login`**), `handled_externally`†, `cancel` | a maintainer's close is a rejection signal |
| `ESC_SHEPHERD_TIMEOUT` | `pr-shepherd-loop` on `converged` | `extend` (new window), `cancel` | |
| `ESC_MERGE_APPROVAL` | `release-loop.ask-merge` | `merge`† (→ merge-check again: the answer is a condition), `cancel` | always when `merge: human` |
| `ESC_ROLLBACK` | `release-loop.ask-rollback` | `rollback`† (the driver refuses the answer when `cmds.rollback` is absent), `extend` (→ ci-main, new window), `keep` (→ close) | optional |
| `ESC_LEARNING_REVIEW` | `factory.ask-learn` when `curation.review: human` ∧ proposals non-empty | `accept_all`†, `select`† (note = a JSON list of proposal ids), `reject_all` | |
| — Execution method, worktree consent, finish menu | PD | — | discard only via `factory cancel --discard` |

Anything a human is asked that is not in a family's registered code table is a bug (§12).

### 5.7 Task sources and the decomposition artifact

Two sources, one interface (`tasksource.Source` grows acceptance arrays, labels, parent, dependencies,
checkpoint, files, credentials — issue #2 D1):

- **Beads** (when `.beads/` exists): all reads/writes through `BdRunner` (`bd ready/show/list/create/
  update/close/dep --json`; verbs, auto-flush, and DB isolation verified and version-pinned in Phase 1),
  executed by the **driver in the driver worktree**, never by an agent. The job's Beads database must be
  isolated to the driver worktree (if `bd` cannot isolate it, `close-beads` moves to `release-loop` after
  merge — a Phase 1 finding, §12.14); **auto-flush is off**; the only flush is inside `beads-commit`, which
  commits the tracked `.beads/issues.jsonl` change with the run trailer and an explicit scope allowance.
  `taskset-load` runs `bd create` for every task id it has not created yet (idempotent by id, so a
  `resplit` reload creates the new ids). `.beads/issues.jsonl` merge conflicts are resolved by the driver
  through `bd` (import theirs, re-export), never by `resolve-conflicts` (`.beads/**` is denied to it):
  **order** — on a conflicting `merge-from-base` the driver first resolves `.beads/**` via `bd`, commits
  that resolution alone, publishes, and only then is the agent briefed to merge the rest (§5.9 step 6).
  In Beads mode `taskset-load` still needs the decomposition document (the source of `bd create`); it
  locates it through the job record (`docs.dir`/`job_id`, or `intake.plan_path`) exactly as
  `record-marker` does. Readiness is re-queried each scheduler iteration; the fold caches only decision
  records. The worktree manager refuses to remove a driver worktree with dirty tracked `.beads/*`.
- **Plan file** (no Beads): exactly one fenced block ```` ```yaml metareview:taskset ```` in the
  decomposition document at `<docs.dir>/<job_id>-decomposition.md` (or `-simple-task.md`) — the path
  `intake` emits as `doc_path` and `author` is briefed to write. `intake.source` is `beads` when `.beads/`
  exists, else `plan_path` when the intake supplied one, else `doc_path`:

```yaml
version: 1
epic: {id: E1, title: "...", intent: "..."}
tasks:
  - id: T1                              # T1..Tn consecutive; a split appends T<n>.1, T<n>.2 … and never renumbers
    title: "..."
    files: {create: [exact/path.go], modify: [exact/other.go], test: [exact/path_test.go]}
    interfaces: {consumes: [], produces: ["func Foo(x int) error"]}
    acceptance:
      - {text: "...", files: [exact/path.go], tests: [TestFoo]}   # files/tests optional; feed the coverage matrix
    depends_on: []                      # earlier ids only
    checkpoint: false
    labels: []                          # e.g. no-tests
    credentials: []                     # ⊆ job.credentials.test names
  - id: T2
    depends_on: [T1]
    ...
```

`taskset_valid` (deterministic, before any lens): ids consecutive with the split convention; `depends_on`
earlier ids only; every `consumes` equals an earlier `produces`; `files.*` exact repo-relative paths,
`create ∪ modify` non-empty, none under a protected path unless named by a signed allowance
(`protected_paths` otherwise); no two tasks without a dependency path share a path in `create ∪ modify`;
no banned placeholder tokens; each task's `credentials` ⊆ `job.credentials.test` (`credentials_missing`);
`acceptance` non-empty; on reload, already-built ids keep their `files`/`acceptance`.

### 5.8 The build unit's contract

`implement` (runner-executed in the clone, `FixEntry`, `MutatesTree`; the brief says uncommitted work is
discarded) returns **claims**:

```
{commits: [<sha>…], status: DONE|DONE_WITH_CONCERNS|BLOCKED|NEEDS_CONTEXT,
 report_path: ".metareview-report/<task>.md", tests_added: [<test id>…], concerns: [<string>…]}
```

`implement` takes `brief_extra: [${verify.failures:-[]}, ${mutate.per_file:-[]}, ${ask-blocked.note:-""},
${ask-escalated.note:-""}]` — the retry-brief channel. `implement.Reduce` emits `concerns[]` as
`Delta.Findings` (source `implementer-concern`) so `triage` (`match-then-adjudicate`) can adjudicate them.
`.metareview-report/` is in the clone's `.git/info/exclude` and outside scope and porcelain checks.

`verify-tdd` (fork, deterministic; `mode: task | suite`; affinity `throwaway`) runs after the §5.9 import
in a **throwaway sandboxed tree** materialised from the ref source at exactly the driver HEAD (never the
agent's working tree; its own sandbox instance; discarded after the receipts), using job cmds:

1. `commits` are ancestors of the job branch tip, later than the task's base, each carrying
   `Metareview-Run: <run-id>`; the imported head equals the driver HEAD — else `implement_unclean`.
2. **Scope:** `git diff --name-only <task.base>..HEAD` ⊆ `files.create ∪ modify ∪ test`; protected paths
   denied unless in `ALLOWED_PATHS` (engine-implicit, §5.3) — else `scope_violated`. In `mode: suite` the
   range is `$JOB_BASE..HEAD` and only the protected-path half applies.
3. **RED/GREEN, engine-run — the build-unit rule, a pure differential:** for each id in `tests_added`
   (`tdd_missing` if empty and the task lacks `no-tests`): the id is a top-level test declared in one of
   the task's changed test files (per-convention: Go `^func <id>\(` — `Test*` functions only, no subtests
   or suite methods; vitest/pytest report file paths);
   at *bare* `task.base` the tree builds and the id is absent or passing; at `task.base` + the head's
   test-file overlay (`mutation.Reproducer`) the run classifies `ClsAssert` **or** `ClsCompile` (accepted
   because bare base builds, HEAD passes, and scope holds); at HEAD the id classifies `ClsPassed`. The
   overlay failure tails are recorded as `red[].fail_before` and concatenated into `fail_before` (exported
   as sha256), passed to the review child as `CONTEXT`. When `Policy.mutation ≠ required`,
   `ClsCompile`-only RED makes `tdd_verified` true with a recorded warning.
4. **Suite:** `test` cmd at HEAD exits 0 and the per-convention report shows ≥ 1 executed test in the
   packages of the task's test files (whole-run and per-package counters per convention, Phase 1);
   stderr/stdout tails are recorded as sha256 + value-scrubbed tails (advisory).
5. **Validation cmds:** `lint`, `typecheck` at HEAD; `coverage` at HEAD parsed by `internal/coverage` (Go
   profile, Istanbul/vitest JSON; pytest-cov XML in 7a): every **production-source** path in `files.create
   ∪ modify` (per the convention's source predicate in `internal/testconv`) must appear with ≥ 1 statement
   and meet `policy.coverage.threshold` per file; non-source paths are outside the per-file rules;
   `no-tests` waives the per-file coverage **and** mutation rules for that task — and because the label is
   agent-authored, `taskset_valid` refuses `no-tests` on a task whose `create ∪ modify` contains a
   production-source path unless the plan was human-approved (`human_approval.plan: required`).
6. **Secrets:** no `job.credentials` *value* appears as a plain substring in `job.base..HEAD`, the report
   dir, or the raw stdout/stderr tails (scanned before scrubbing) — else `secret_in_diff`; also checked by
   `push` and before any value reaches a var, brief, or GitHub-bound body. Encoded or split values are out
   of scope (a tripwire, not a control; §14).
7. Mints receipts (`sourceRevision = HEAD sha`); receipts in `implement`'s output are ignored. Atoms:
   `tdd_verified`, `scope_respected`, `suite_green`, `lint_ok`, `typecheck_ok`, `coverage_ok`,
   `secrets_clean`; `implement_accepted` = all applicable; `implement_rejected` = ¬accepted. In `mode: suite`
   step 3, the allow-list half of step 2, and the per-file half of step 5 are skipped (each task's
   `verify` already applied them; the whole-suite `policy.coverage.threshold` still holds). Every failed
   check is also emitted as
   `failures[{check, file, line, desc}]` (file/line from the tool's report where it names one, else the
   task's first changed file and line 0) — the shape the goldens grammar accepts, so a fix child receives
   the deterministic failures as goldens.

Then the review child `fix-loop-proved-clean` over `task.base..HEAD` (goldens `triage.confirmed`; `CONTEXT`
= `fail_before`), then **`mutate`** (`mutate_cmd`, throwaway tree): `mutation_ok` requires
`Score.Complete()`, ≥ 1 mutant in every production-source path of `files.create ∪ modify`, and — under
`required` — zero survivors and zero uncovered sites in those paths; `advisory` records; `off` skips.
**`prove` is not a node of the `build` family** (decision §12.10, accepted; the review child's bug-family
`prove` is unchanged). Then `record-marker`, then `task-close` (Beads DB + run store; no tree commit).

**`record-marker`** (fork, deterministic, affinity `driver`; params `scope`, `child`) reads the child's
leaf fold and writes the marker with `headSha` = **the `Head` of the leaf's last transition out of a
`review-lenses` or `match-then-adjudicate` node**, valid only if every confirmed bug of the leaf is either
fixed (`Status`) or covered by a `disposition` record (incl. policy `advisory`) — the marker carries
`DispositionedFindingIDs` and the signer; `verdict` = `PASS` when none, `PASS_ADVISORY` otherwise;
`fromFsmRunId = leaf`; `executionMode = subagent-adjudicated` when every `review-lenses` node ran via
`runner`/`subagent`, else `in-session-emulated`. For `scope: epic-ready` it also computes the **acceptance
coverage matrix** (acceptance item → its `files`/`tests` refs → engine receipts → review child) from the
taskset it locates through the job record (`docs.dir`/`job_id` or the Beads epic) — an intake with no
taskset (`entry_ship`, `pr_number`) yields an explicitly recorded `matrix{absent: true}`, never a vacuous
pass — and outputs
`matrix{covered, uncovered[], absent}`; `marker_recorded` requires every cell covered or dispositioned
(an absent matrix records `absent: true` and mints `PASS_ADVISORY`), else `matrix_uncovered`. Refuses
`mock` (`ERR_MARKER_MOCK`), a missing child (`ERR_MARKER_NO_CHILD`), and a leaf head that is **not
current at HEAD** under the marker-currency rule of §5.3 (`ERR_MARKER_STALE`) — a review-only child
leaves no commit, and a `gate-review` log commit touches only excluded paths, so both keep the marker
current. Readers (`LatestReviewEvidence`, `validateFromRunDiff`, `record-lenses`, `merge-check`,
`marker-check`) all apply that one rule; receipts keep "ancestor necessary, exact match
sufficient".

### 5.9 Trees, roots, and the sync protocol

**Two roots**, stated per file:

| Content | Root | Examples |
|---|---|---|
| **Transient state** | `state.Dir()` = `METAREVIEW_STATE_DIR`, default `<main worktree root>/.metareview` | `runs/`, `runs.jsonl` (incl. markers), `findings.jsonl`, `shards/`, `jobs/`, `models.yaml`, `adapters/`, `sandbox/`, `git-hooks/`, `refsrc/` |
| **Checkout content** | the worktree/clone toplevel the command runs in | the reviewed diff, `docs/metareview/*`, the committed knowledge table (§5.11), the tracked `.metareview/{calibration.jsonl, learning-runs.jsonl}` exceptions, `.beads/*`, `SERVICE_INVENTORY.md` |

`state.Dir()` is the single resolver every transient reader/writer uses. Split-state detection looks for
ephemeral files (`runs/`, `runs.jsonl`, `findings.jsonl`, `shards/`) in a worktree's `.metareview/`;
`metareview state migrate --from <worktree>` rewrites `repoRoot` in JSONL rows and moves **terminal,
non-mock** run directories verbatim (others must be finished or `--discard`ed).

**Two trees per job** (serial default, Phases 1–6), with **tree affinity** per kind (§7):

- the **agent clone** — `git clone --shared <main> .worktrees/<job>-agent` (alternates → the main repo's
  `.git/objects`, mounted read-only), on the job branch; its `.git` is inside the agent sandbox; every
  agent process, every declared cmd, and **every git operation on the clone** run as sandboxed
  subprocesses — the driver never executes `git`, a package manager, or anything else with the clone as
  `cwd`/`--git-dir`, and never checks agent commits out with global git config (verified live, §15);
- the **driver worktree** — `git worktree add .worktrees/<job>-driver <job-branch>`; the driver owns
  `refs/heads/<job-branch>`, runs `bd`, `merge-from-base`, `beads-commit`, `curate-commit`, `push`,
  markers, and reviews here. **Driver-side git always runs with `GIT_CONFIG_GLOBAL=/dev/null`
  `GIT_CONFIG_NOSYSTEM=1` (identity via `-c user.*`), and diffs with `--no-ext-diff --no-textconv`**, so no
  filter/textconv/hooksPath driver exists to be selected by agent-committed attributes; the engine's
  `RealExec` flags change accordingly (`-c diff.external=` is dropped — it makes any `diff=` attribute
  fatal). Because `gate.scrubEnv` drops every `GIT_*` variable, the hardening is set **inside** `RealExec`
  and on the env of every subprocess that execs git itself (`gate-review`'s gate commands, `bd`), which
  also scrubs `METAREVIEW_ALLOW_MECHANICAL_PASS`. The three network operations — `push`, the base-tip
  `fetch`, and `sync-branch`'s fetch of `origin/<job-branch>` (always `-c fetch.fsckObjects=true`) — use a
  **driver-owned minimal config** (`GIT_CONFIG_GLOBAL=<state-dir>/gitconfig`, holding
  only `credential.*` — e.g. `credential.helper=!gh auth git-credential` — and the `core.hooksPath`
  `setup --install-hooks` materialised, so the pre-push gate fires however the user installed hooks),
  never the user's global file.
  **Every driver-side read or write of a path inside a worktree** (`review` logs and context packs,
  `FINDINGS.md`, the knowledge table, `.beads/*`, `.metareview-report/*`, the `--evidence` file) walks
  the path with `Lstat` and opens with `O_NOFOLLOW` (the `sidecar`/`export` pattern), refusing any
  symlink component — git checks agent-committed symlinks out, and the driver must never write or read
  through one.

**Sync protocol (the only crossings):**
1. *Publish (driver → clone).* The driver writes the job branch head and the fetched base tip
   (`refs/metareview/base-tip`, the merge target — distinct from `$JOB_BASE`, the diff anchor) into a
   read-only **ref source** `<state-dir>/refsrc/<job>/` (a bare, hook-free
   ref file over the main `.git/objects` alternates), mounted read-only into the sandbox; recorded as a
   `tree-publish` record.
2. *Mirror (before every agent step or sandboxed cmd).* A sandboxed subprocess in the clone runs `git
   fetch <refsrc> +refs/heads/<job-branch>:refs/metareview/mirror-head
   +refs/metareview/base-tip:refs/metareview/base-tip && git checkout -B <job-branch>
   refs/metareview/mirror-head && git clean -fd -e .metareview-report/` (recorded as a `tree` event) —
   **forced** destination refspecs: a bare `<ref>` only writes `FETCH_HEAD`, the checkout must name a ref
   rather than a line order, and every restore (step 4) moves the branch *backwards*, which an unforced
   refspec rejects as non-fast-forward. The clone always starts from the driver head (ignored files
   persist by design; gate inputs come from throwaway trees).
3. *Import (after every agent step, inside the same `advance`, before the tree re-read).* With the
   **driver worktree** as `cwd`: `git -c fetch.fsckObjects=true fetch --upload-pack '<sandbox-wrapper>
   git-upload-pack' <clone-path> refs/heads/<job-branch>` (upload-pack runs *inside* the sandbox, so
   agent-controlled alternates/packs are parsed with agent privilege only); then **before any checkout**,
   `git diff --raw --no-renames <driver-head> FETCH_HEAD` (runs no drivers; `--raw` alone — `--name-status`
   would override it and hide the mode columns) must not (a) touch a protected path (§5.12.4 — basenames
   match at any depth, case-insensitively on case-insensitive filesystems; `docs/metareview/**`,
   `.beads/**`, and the knowledge table are protected here because the driver writes them and the agent
   never needs to) nor (b) add, modify, or type-change any entry whose **destination mode is `120000`**
   (a symlink) anywhere, unless named by a signed allowance — else `ERR_IMPORT_PROTECTED` (driver `import-park` on
   `ESC_PROTECTED_PATHS`). A protected path is exempt from (a) — never from the symlink condition (b),
   which no allowance short of a signed one waives (a symlink and a regular file can share a blob) —
   when its blob at `FETCH_HEAD` equals its blob at `refs/metareview/base-tip` (the base branch itself
   changed it — the `resolve-conflicts` case), so legitimate base changes never train blanket allowances
   — **except the driver-resolved set** (`.beads/**`,
   the knowledge table, `docs/metareview/**`), whose blobs must equal the *driver head's* (no base-tip
   exemption: taking the base's `issues.jsonl` would silently drop the job's closes). Then ancestry check
   and `git merge --ff-only FETCH_HEAD`. Non-fast-forward → the refused `FETCH_HEAD` is parked under
   `refs/metareview/discarded/…` and `import_diverged` (retryable) is raised.
4. *Restore (retry reset, child retry).* The driver moves `refs/heads/<job-branch>` to the target head
   (parking the discarded head), publishes, and the clone mirrors.
5. *Throwaway trees.* `verify-tdd`/`validate`/`mutate`/`health`/`rollback` and the `Reproducer`'s temp trees
   are materialised **from the ref source** inside a fresh sandbox instance whose writable set is that
   tree, a per-invocation `HOME` (with `TMPDIR` inside it), and the **job dependency cache**
   `<state-dir>/jobs/<job>/cache/` (`GOMODCACHE`, `GOCACHE`, the npm/pnpm/yarn cache, pip's cache — env
   pinned by the convention), with `refsrc/` and the main `.git/objects` mounted read-only: `git clone
   --shared --no-checkout <refsrc> <tree> && git -C <tree> checkout --detach <sha>` (writes only inside
   the tree; `git worktree add` cannot be used — it writes `$GIT_DIR/worktrees/` into the read-only
   source), at the imported head (or `$JOB_BASE` for `health`/`rollback`). A clone holds committed content
   only, so **before the gate cmds run, `cmds.install` runs inside every throwaway tree at its own head**
   (convention default when undeclared, each with a fixed, consented egress list: `go mod download` →
   `proxy.golang.org`, `sum.golang.org`; `npm ci --prefer-offline` → `registry.npmjs.org`; `pip install
   -r` → `pypi.org`, `files.pythonhosted.org`; a declared `install` uses its own `egress:`; the only cmd
   exempt from the "`egress:` cmds run at `$JOB_BASE`" rule) — the cache makes it offline and near-free
   after the first run. It is skipped only for conventions whose install target lives outside the tree
   (Go: the module cache) and the lockfile hash is unchanged; `node_modules`/site-packages live *in* the
   tree, so vitest/pytest install in every throwaway (from the warm cache). The cache is job-scoped and
   disposed with the job; it is mounted **writable only in the credential-free `install` invocation** and
   **read-only in every credential-bearing invocation** (§5.12.1's writable set — the tree plus `HOME` —
   is exact), so no sandbox invocation ever holds both `job.credentials` and network egress and nothing
   written under the credentials can reach an install's allowed hosts; poisoning by test code is thereby
   impossible, not merely job-scoped. The refsrc's
   alternates file names the main `.git/objects` by absolute path, so the sandbox binds it at that path.
   The tree is discarded after the receipts; `job.credentials` exist only there. `mutation.Reproducer`
   gains a materialiser seam for this.
6. *Conflicts.* `merge-from-base` runs in the driver worktree; on conflict the driver aborts the merge
   and **merges the whole driver-resolved set itself**: for every path under `.beads/**`, the knowledge
   table, and `docs/metareview/**` it takes the three-way result — clean auto-merges and base-side
   additions as they are, `.beads/**` conflicts through `bd` (§5.7), knowledge-table conflicts by union
   (the table is append-only), `docs/metareview/**` conflicts by keeping both sides (run-named files never
   collide) — and commits that alone, so the driver head already holds the base's rows and logs (nothing
   of main's is reverted); it records the remaining conflicting paths, publishes
   (`refs/metareview/base-tip` = the fetched base tip; the `tree-publish` record lists
   `driver_resolved[]`), and `resolve-conflicts` (runner, clone) is briefed to `git merge
   refs/metareview/base-tip` inside the sandbox, restore the driver head's blobs for the whole
   driver-resolved set (`git checkout refs/metareview/mirror-head -- .beads/ <knowledge table>
   docs/metareview/ && git add` — stage-independent; `--ours` silently skips non-conflicted paths, and a
   three-way merge re-runs from the same merge-base and re-conflicts on those paths, both verified
   live), and resolve the rest by hand; the result is imported as any agent step, where the
   driver-resolved set equals the driver head by construction (step 3).

Receipts are minted only when the imported head equals the driver HEAD. `task.base` = the job branch head
at scheduling; the epic integration diff = `$JOB_BASE..HEAD` (§5.3: `JOB_BASE` is a re-resolved
merge-base SHA).

`internal/worktree` implements the Superpowers rules as code: detect existing isolation; create under
`.worktrees/` (excluded via `.git/info/exclude`) from `branch.template` (never on `main`/`master`:
`on_default_branch` at `load` is an explicit failure); dependency install as a sandboxed subprocess in the
clone (the optional `cmds.install`, registry egress through the proxy allow-list); baseline green (an
engine receipt; red → `ESC_BASELINE_RED`); remove only registered trees it created, from the main root,
after every run that used them is terminal and tracked `.beads/*` is clean, then `prune`. Per-task clones
and `integrate-branch` are 7a. `fsm advance --from --auto-worktree` performs the `git worktree add
<checkpoint-head>` the docs currently ask the human for.

### 5.10 GitHub surface

`internal/github` with an injectable `GhRunner`. **Reads:** `pr-observe` — GraphQL `pullRequest{state,
mergeable, reviewDecision, headRefOid, reviews{author{login}, state, submittedAt, commit{oid}},
reviewThreads(first:100){nodes{id, isResolved, isOutdated, path, line, comments{author{login},
authorAssociation, body, url, createdAt, replyTo{author{login}}}}}}`, `gh pr checks --json`, failing-check
logs (`gh run view --log-failed` for "ours"), branch protection, `gtg --format json` when installed. It
**blocks** (up to `SHEPHERD_OBSERVE_MAX`, default `min(SHEPHERD_TIMEOUT / 2, max_runner_timeout)`,
polling every `SHEPHERD_POLL` on the `Clock` seam) until the normalised state changes, emitting one output per change; reaching the
deadline unchanged emits `{pending: true}` (exit 0, not a `timeout` error) so `converged:wall_clock` can
route on the loop edge. Iterations are per change, not per poll. Because the run lock is held while it
blocks, `factory cancel` first signals the driver (pid in the job record), which kills the subprocess,
before `fsm cancel` takes the lock:

```
PrState{number, pr_state: OPEN|CLOSED|MERGED, head_sha, mergeable, review_decision,
        approvals[{login, trusted, commit_sha}],          # each login's LATEST review only
        protection: {requires_non_bot_review, dismiss_stale},
        checks[{name, state, bucket, url, ours, log_tail_sha256}],
        failing_findings[{file, line, desc}],             # parsed from "ours" logs; goldens for fix
        gtg: {status, action_items[]}|null,
        threads[{id, path, line, resolved, outdated, url, comments[{login, association, trusted, body_hash, reply_to_trusted}],
                 trust: trusted|untrusted|mixed, severity, disposition}],
        protected_paths[], pending, closed_by}
```

`ours` = the failing check ran on `head_sha` and its log names a file in the job diff. Thread `trust` =
`trusted` iff every comment is trusted (§5.12.4); `mixed` threads are never auto-resolved. `branch-observe
{ref}` returns `{ref, head_sha, checks[]}` and blocks likewise. **Shepherd atoms, in evaluation order
(the machine takes the first true edge, so each is defined to exclude the earlier ones):** `pr_merged`
(`pr_state == MERGED`); `pr_closed` (`CLOSED`); `ci_failing_ours` (a failed check on `head_sha` with
`ours`); `ci_failing_foreign` (a failed check, none `ours`); `conflicting` (`mergeable == CONFLICTING`);
`threads_actionable` (an unresolved `trusted` thread not yet dispositioned this iteration);
`threads_untrusted_only` (unresolved threads exist, none trusted); **`ready`** = `pr_state == OPEN` ∧
every check on `head_sha` succeeded or is neutral ∧ no unresolved thread of any trust ∧
`review_decision ≠ CHANGES_REQUESTED` (a body-only rejection with no thread is `threads_untrusted_only`
for the human, never a `release`↔`ship` ping-pong) ∧ `mergeable ≠ CONFLICTING` ∧ (`gtg == null` ∨ `gtg.status == READY`); `pending` = none of the above (a check still
running, or the observe deadline). Within one precedence class (§5.3) edges are evaluated in listed
order, which is why the outline lists them in this order. `push_rejected` = `push` output present ∧
`pushed_sha == ""` ∧ `protected_paths == []` ∧ ¬`dry_run`; `conflicts_unresolved` = `resolve-conflicts`
output present ∧ ¬`resolved`; `replied`/`resolved`/`merged`/`pr_exists` = output present ∧ ¬`dry_run`
(an empty list is still a success — a `fix_now` triage may have no replies); `merge_conditions_met` is
also true when `pr_state == MERGED` (a human merged in the UI; `do-merge` then reports
`already_merged`).

**Writes** are kinds with **fixed argv templates**, object-scoped to the job, consented by
`job.github.writes`, each output carrying `dry_run`: `open-pr` (idempotent — returns the open PR for the
branch; on a CLOSED PR the job opened it reopens it **only when the close actor was `factory_login`**,
otherwise the `retry` answer at `ask-closed` is privileged (§5.6) and a signed answer creates a new one;
records the PR number in the job record), `pr-reply {thread_id, body via stdin}` (≤ 1 per thread per
cycle; one reply per `filed[]` entry that names a `thread_id` — the triage body if present, else a
template line with the issue URL — so a filed thread is always in `replied`; output `{replied[],
dry_run}`), `pr-resolve {goldens, fixed, sources, replied}` (params: the fix child's concatenated
`goldens[]`, its `fixed_golden_idx` and `golden_sources` — all three from one child output — and the
threads `pr-reply` just answered; the driver resolves the `thread_id`s of the `$GOLDENS`-segment goldens
(each carries its `comment-triage` finding's `thread_id`) whose index is in `fixed`, **and** every
`trusted` thread in `replied` (reply-then-resolve, as gtg expects — an open trusted thread blocks
`ready`); a `mixed`/untrusted thread is never resolved; output `{resolved[], dry_run}`), `push` (`git
push origin <job-branch>:<job-branch>` — never force, never
`--no-verify`, refused for `main|master`; protected-path check over `merge-base(origin/<base>, HEAD)…HEAD`
→ `protected_paths[]`; `pushed` = `pushed_sha ≠ ""` ∧ `protected_paths = []` ∧ ¬`dry_run`; a remote
refusal is `push_rejected` with `stderr_sha256`), `merge` (the job's PR), `file-issue {items}` (one issue
per item of the `comment-triage.file_issue` list, idempotent per thread — it searches for an open issue
whose `Metareview-Origin:` trailer names the thread URL and returns it instead of filing twice; label
`factory:filed`, trailer `Metareview-Origin: <url>`; output `{filed[{thread_id?, number, url}], dry_run}`), `close-issue` (numbers in the job record
only). Replies carry metaswarm's attribution line. `comment-triage` routes are exclusive: `fix_now` =
`findings ≠ []`; `file_issue` = `findings == [] ∧ file_issue ≠ []`; `reply_only` = otherwise (`replies`
may be non-empty on every route).

### 5.11 Knowledge, priming, handoff

- **One knowledge table.** `knowledge.Path(root)` = `.beads/knowledge/metareview.jsonl` when `.beads/`
  exists, else `.metareview/knowledge/metareview.jsonl` (both are the tracked exception today); `learn`,
  `curate-commit`, and `knowledge.Collect` share it, and `Collect` reads **both** roots so existing
  `.beads/knowledge/*` rows are preserved. Rows are appended with `supersedes`; readers filter.
- **Priming:** every adapter payload carries `knowledge[]` (facts filtered by the node's files/keywords),
  **nonce-fenced as untrusted**.
- **Curation, two nodes.** `curate` (runner, `CURATOR`, reads only) proposes `accepted[{fact, supersedes[]}]`
  from comments by **non-bot trusted logins** (bot comments are evidence for a human batch only);
  `curate-commit` (driver, `MutatesTree`) materialises the rows the answer allows into the knowledge table
  in the driver worktree (`supersedes` restricted to rows the same job wrote unless a human approved the
  batch) and commits with the run trailer — before `finalize`, so they ride the PR (S15).
- **Post-merge learning:** the `learn` node in `release-loop` runs `learn --post-merge` over the merged PR
  and writes to the **state dir only** (`<state-dir>/learning/<job>/`); the sandbox cannot read the state
  dir, so the driver folds every *pending* proposal there into the next job's `curate` envelope as a
  nonce-fenced `proposals[]` field, and that job's `curate-commit` carries accepted rows into its PR.
  Pending = no `consumed {by_job, disposition, at}` marker in that directory. The marker is a projection
  of the run store, not an independent fact: the driver writes it from `curate-commit`'s node output
  (which names the proposal directories it consumed) or the `reject_all`/`curation_auto` transition, so a
  crash between the commit and the marker is repaired by `factory resume` from the run; `curate-commit`
  deduplicates rows by fact text against the table; and `factory cancel` or a PR closed after
  `curate-commit` rewrites the marker as `disposition: unmerged`, which makes the batch pending again —
  so a proposal is carried into exactly one *merged* PR. No learn branch. Until Phase 5b the `learn` kind outputs `{learned: false}`.
- **Handoff = export:** `factory export <job>` writes the bundle plus a generated `HANDOFF.md`; `factory
  resume <job>` is the one sentence.

### 5.12 Trust model for an autonomous host

A Phase 0a/0b deliverable; the platform jobs and every **smoke** exit run behind it (scripted exits use
fake adapters and may run `--unsafe-no-sandbox`). The threat is a prompt-injected agent holding shell
access, and agent-authored content (tests, hooks, config, **attributes**) executed or interpreted by
anything with more privilege.

1. **Two trees, one crossing.** Everything that is, or runs, agent-authored content — runner adapters,
   every declared cmd, throwaway trees, dependency installs, and every git operation on the clone
   including `git-upload-pack` — runs inside an OS sandbox whose writable set is the clone (its `.git`
   included) plus a scratch `HOME`; read-only mounts: the main `.git/objects`, `<state>/adapters`,
   `<state>/sandbox`, `<state>/refsrc/<job>`. Not mounted: the main `.git` otherwise, the driver worktree,
   the rest of the state dir, `~/.ssh`, `~/.config/gh`, `~/.claude`, `~/.codex`, `~/.npmrc`, `~/.netrc`,
   `~/.aws`. The driver's only contact with the clone is the sandboxed-upload-pack `fetch` of §5.9 step 3,
   and it never checks out agent content with global git config. Credential-bearing cmds run in **their own
   sandbox instance** whose writable set is exactly their throwaway tree plus a per-invocation `HOME`, with
   network off unless the cmd declares `egress:`; test code is attacker-controlled by definition.
2. **Egress.** The sandbox denies all network. Runner adapters and `egress:`-declared cmds (`health`,
   `rollback`, and the package manager during dependency install — every tree file their argv executes is
   pinned in the consent digest, else `ERR_CMD_UNPINNED_EGRESS`; `egress:` cmds run at `$JOB_BASE`) reach
   hosts only through a **driver-owned egress proxy**: one unix socket **per sandbox invocation**, mounted
   into that sandbox only, bound to exactly that invocation's allow-list (host + path prefix), reached via
   the in-sandbox `egress-bridge` on `127.0.0.1:<ephemeral port>` and the drivers' `*_BASE_URL`; the
   bridge requires a per-invocation token (macOS loopback is shared across processes) and each profile
   allows only its own port; on Linux the sandbox runs `--unshare-net` and brings `lo` up itself (a fresh
   netns has loopback down); the proxy injects provider credentials for runner sockets only; every
   request is audited into the job's `usd`/token accounting.
   **No API key or OAuth token is inside any sandbox.** Mechanisms: macOS `sandbox-exec` (deprecated but
   functional) or a container; Linux `bwrap` (unprivileged userns) or a container; deny-default profiles.
   A **sandbox self-test** at job start (writes to the state dir, main `.git`, driver worktree denied;
   **reads** of `~/.ssh`, `~/.config/gh`, `~/.claude`, `~/.netrc`, `~/.aws`, the state dir denied;
   non-allow-listed connect denied; the bridge reachable on loopback with its token and refused without;
   clone write allowed) is recorded in the job record; `sandboxed: true` only on pass. `--unsafe-no-sandbox` stamps `false` and disables every GitHub write and merge (§12.6).
3. **Engine-derived evidence and consent.** Nothing an agent emits is a gate input except as a claim the
   engine re-derives (§5.8, §5.3); `author.base` is re-derived from the `tree-publish` record preceding the
   step. Markers are written only by `record-marker`. Rubrics, templates, and sandbox profiles are
   **embedded** and addressed by name; `author.template` is an open registry (§14). Consent digests are in
   `job.yaml` plus signed `consent-accepted` rows; the driver never supplies `--accept-workflow-change`.
4. **Untrusted text and protected paths.** Diffs, issue bodies, PR comments, review bodies, knowledge rows,
   `CONTEXT` are nonce-fenced in every payload. **Trust is per comment:** trusted iff `authorAssociation ∈
   {OWNER, MEMBER, COLLABORATOR}` or login ∈ `trusted_authors`, or login ∈ `trusted_bots` *and* the comment
   is not a reply to an untrusted login. Untrusted comments are never auto-fixed or auto-resolved. Issues
   carrying `factory:filed` or `Metareview-Origin:` are refused as intents (`ERR_INTAKE_UNTRUSTED`) until a
   human removes both. **Protected paths** — `.github/**`, `.gitattributes`, `.gitmodules`, `.lfsconfig`,
   `.git*` (any depth), any `core.hooksPath`, `rubrics/**`, `workflows/**`, `.metareview/**` (incl. the
   tracked exceptions — driver-written), **`docs/metareview/**`**, `.beads/**`, the knowledge table, and
   **any symlink** — are denied to `taskset_valid`, `scope_respected`, the import, and `push` unless a
   signed `ESC_PROTECTED_PATHS` answer whose payload names `paths[]` allows them; the allowance is
   engine-implicit `ALLOWED_PATHS` everywhere (§5.3). The driver-written set is protected because
   `docs/metareview/**` is diff-excluded — no lens would ever see a symlink or a forged log planted there
   — and because the driver writes into it with its own privilege (§5.9: `Lstat` + `O_NOFOLLOW`).
5. **Merge needs an external condition bound to the head.** `merge-check` requires `head_sha == HEAD`
   (a foreign commit on the job branch is `conditions_failed`; the shepherd's `sync` state then imports
   it under the step-3 checks and `enter` finds the markers stale) ∧ gtg READY ∧ pr-ready and epic-ready markers with a **pass verdict**
   (`PASS` or `PASS_ADVISORY` — every dispositioned or taskset-less job mints the latter), both **current
   at HEAD** under §5.3's marker-currency rule (for a taskset-less intake the epic-ready marker records
   `matrix.absent` and `gate-review` skips that scope) ∧ every trusted thread resolved ∧ no `mixed`/untrusted
   unresolved threads pending disposition ∧ `review_decision == APPROVED` ∧ **one of**: the *latest* review
   by a `trusted_authors` login ≠ `factory_login` is APPROVED with `commit_sha == head_sha`, or a signed
   `ESC_MERGE_APPROVAL` answer whose payload names `head_sha` (consumed by `merge-check` on the next
   evaluation). Any push after an approval invalidates it. `merge: auto` additionally requires
   `protection.requires_non_bot_review` (`ERR_JOB_REPO_UNPROTECTED` at job start); `merge: human` needs no
   protection (§12.13). Refused when `METAREVIEW_ALLOW_MECHANICAL_PASS` is set or when unsandboxed.
6. **Answers.** Factory-raised escalations carry actor `factory:<job-id>`. **Unprivileged** choices —
   exactly `retry`, `context`, `revise`, `answer`, `adjust`, `provided`, `skip_tasks`, `skip_blocked`,
   `split`, `amend_plan`, `retry_narrowed`, `treat_as_real`, `fix_code`, `keep`, `reject_all`, `extend`,
   `cancel` — accept the local CLI with `--by` as audit metadata (one exception: `retry` at
   `ESC_PR_CLOSED` is privileged unless `closed_by == factory_login`). **Every other choice is privileged**
   (`approve`, `override`, `proceed`, `allow`, `continue`, `waive`, `dismiss`, `handled_externally`,
   `merge`, `rollback`, `accept`, `accept_all`, `select`) and requires a signed answer file: `ssh-keygen -Y
   sign -n metareview-answer` over canonical JSON `{job_id, esc_id, choice, note_sha256, head_sha,
   not_after, paths?}`, verified at `advance` against the job's `allowed_signers`; an `esc_id` is
   single-use; `head_sha` must equal HEAD.
7. **Redaction.** `process_call` keeps `{runner, model, effort, attempt, duration_ms, tokens}`; adapter
   stdin/stdout are hashes plus the validated output; every registered kind has an export rule; free-text
   output fields (`await.note`, `intake.intent`, `implement.concerns`, `author.questions`, triage bodies,
   `fail_before`, test tails) are exported as sha256 — the reflection test that pins the redaction map is
   extended to `NodeOutputData` and `needs_input`.
8. **Outputs as paths/refs** are validated (§5.3 grammar; commits verified as ancestors); worktree removal
   is registration-checked; runner ladders may not leave `allowed_providers`; `curate` proposes only.

---

## 6. Workflows

Grammar of §5.3. Each list is `from --gate--> to [outcome] [loop]`; every await lists every choice;
model refs are classes; `cancelled` is implicit; `@from` is the entering state. `sdlc-loop*` and
`review-loop` are unchanged; the rename is Phase 7b (decision §12.1, accepted).

**`fix-loop-proved-clean`** (family `bug`, Phase 1; vars as `sdlc-loop-proved` + `CONTEXT`) —
`sdlc-loop-proved` with `recheck` replacing the `still-present` verify (a fresh review subsumes it):
```
discover(review-lenses, context:$CONTEXT)  --findings_empty--> done[clean] ; --findings_nonempty--> adjudicate
adjudicate(match-then-adjudicate)          --confirmed_empty--> done[clean] ; --confirmed_fixable--> fix ; --confirmed_unfixable_only--> done[reviewed]
fix(agent-edit)                            --commit_exists--> prove
prove(prove, test_cmd:test)                --pins_proven--> recheck ; --pins_unproven--> recheck
recheck(review-lenses)                     --findings_empty--> done[clean] ; --findings_nonempty--> discover (loop)
convergence: any: [{max_iterations: 8}, {budget: {tokens: N}}]   # bug family: terminal overflow as today
```

**`pr-review-loop`** (family `bug`, Phase 2) — `review-loop` with `discover(review-lenses,
rubric:rubrics/pr-ready-review-rubric.md, context:$CONTEXT)`; review-only (`clean | reviewed`).

**`task-build-loop`** (family `build`, Phase 1; vars `TASK`, `BASE`, `BRIEF`, `SOURCE`, `NOTE` default
`""`, `GOLDENS` default `[]` — the task's open projected findings on a `retry_narrowed`; `ALLOWED_PATHS`
is engine-implicit; `outputs: {confirmed: ${review.confirmed:-[]}, unverified: ${review.unverified:-[]},
plan_mandated: ${review.plan_mandated:-[]}, advisory: ${review.advisory:-[]}, fixed_golden_idx:
${review.fixed_golden_idx@all:-[]}}`):
```
implement(implement, max_node_attempts:3, brief_extra:[…, $NOTE])  --implement_done--> verify ; --implement_blocked--> implement-ctx ; --node_exhausted--> ask-tool
implement-ctx(implement, context:expanded)            --implement_done--> verify ; --implement_blocked--> implement-tier ; --node_exhausted--> ask-tool
implement-tier(implement, model:$PLANNER)             --implement_done--> verify ; --implement_blocked--> ask-blocked ; --node_exhausted--> ask-tool
ask-tool(await ESC_TOOL_FAILURE)                      --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
ask-blocked(await ESC_TASK_BLOCKED)                   --chose:context--> implement (loop) ; --chose:split--> done[split] ; --chose:cancel--> cancelled
verify(verify-tdd, mode:task)                         --implement_accepted--> triage ; --implement_rejected--> implement (loop) ; --converged--> ask-escalated
triage(match-then-adjudicate)                         --node_output_present--> review
review(child-workflow, workflow:fix-loop-proved-clean, base:$BASE, goldens:[$GOLDENS, @from.context.findings, @from.context.bugs, ask-escalated.context.bugs, triage.confirmed], vars:{CONTEXT:${verify.fail_before:-""}})
   --child_pass--> plan-check ; --child_reviewed--> plan-check ; --child_escalated--> ask-escalated ; --child_cancelled--> cancelled
plan-check                                            --plan_conflict--> ask-plan ; --plan_clear--> unverified-check
ask-plan(await ESC_PLAN_CONFLICT, context:{findings:${review.plan_mandated:-[]}, brief:$BRIEF})  --chose:fix_code--> review (loop) ; --chose:amend_plan--> done[split] ; --chose:waive--> unverified-check
unverified-check                                      --unverified_block--> ask-unverified ; --unverified_clear--> mutate
ask-unverified(await ESC_UNVERIFIED_FINDINGS, context:{bugs:${review.unverified:-[]}})  --chose:dismiss--> mutate ; --chose:treat_as_real--> review (loop) ; --chose:cancel--> cancelled
mutate(mutate, mutate_cmd:mutate)                     --mutation_ok--> mark ; --mutation_survivors--> implement (loop) ; --converged--> ask-escalated
mark(record-marker, scope:task-done, child:review)    --marker_recorded--> close
close(task-close)                                     --node_output_present--> done[built]
ask-escalated(await ESC_TASK_ESCALATED, context:{bugs:${@from.confirmed:-[]}})  --chose:retry_narrowed--> implement (loop) ; --chose:split--> done[split] ; --chose:override--> close ; --chose:cancel--> cancelled
convergence: any: [{max_iterations: 4}, {budget: {tokens: N}}]   # windows re-arm on every answer
```
(`plan-check`/`unverified-check` are node-less states reading `review`'s verified output, `Policy`, and
`Records.disposition`. `review` is a merge point: entered from `triage` its `@from.context.*` sources are
`[]` and `triage.confirmed` is the goldens list; entered from `ask-plan`/`ask-unverified` the await's
echoed `context` supplies the findings the human chose to act on. The `context` note of `ask-blocked`/
`ask-escalated` reaches `implement` through `brief_extra`.)

**`epic-build-loop`** (family `taskset`, Phase 2; vars `SOURCE`, `DOC_PATH` default `""`, `INTEGRATE`
default `true`; `outputs: {advisory: ${schedule.advisory:-[]}, confirmed: ${integrate.confirmed:-[]}}`):
```
load(taskset-load, source:$SOURCE)        --load_ok--> schedule ; --baseline_red--> ask-baseline ; --credentials_missing--> ask-creds ; --protected_paths--> ask-protected ; --taskset_invalid--> failed[failed] ; --on_default_branch--> failed[failed]
ask-baseline(await ESC_BASELINE_RED)      --chose:proceed--> schedule ; --chose:cancel--> cancelled
ask-creds(await ESC_CREDENTIALS_REQUIRED, context:{names:${load.credentials_missing:-[]}, tasks:${load.tasks_missing_credentials:-[]}}) --chose:provided--> load (loop) ; --chose:skip_tasks--> issue-skipped ; --chose:cancel--> cancelled
ask-protected(await ESC_PROTECTED_PATHS, context:{paths:${load.protected_paths:-[]}})  --chose:allow--> schedule ; --chose:cancel--> cancelled
issue-skipped(file-issue, template:skipped-task, items:${@from.context.tasks:-[]}) --node_output_present--> schedule (loop) ; --converged--> ask-task
schedule(next-ready-task)                 --task_ready--> build ; --all_tasks_done_integrate--> integrate ; --all_tasks_done_plain--> done[built] ; --deadlock--> ask-deadlock
ask-deadlock(await ESC_DEP_DEADLOCK, context:{tasks:${schedule.blocked_ids:-[]}})  --chose:skip_blocked--> issue-skipped ; --chose:cancel--> cancelled
build(child-workflow, workflow:task-build-loop, base:${schedule.base}, vars:{TASK:${schedule.task_id}, BASE:${schedule.base}, BRIEF:${schedule.brief_path}, SOURCE:$SOURCE, NOTE:${@from.note:-${ask-checkpoint.note:-""}}, GOLDENS:${@from.context.bugs:-[]}})
   --child_built_checkpoint--> ask-checkpoint ; --child_built--> inventory ; --child_split--> resplit ; --child_escalated--> ask-task ; --child_cancelled--> cancelled
ask-checkpoint(await ESC_CHECKPOINT)      --chose:continue--> inventory ; --chose:adjust--> inventory ; --chose:cancel--> cancelled
inventory(inventory-update)               --node_output_present--> schedule (loop) ; --node_exhausted--> ask-tool ; --converged--> ask-task
resplit(author, template:split-task, path:$DOC_PATH, task:${schedule.task_id}, note:${@from.note:-""}) --doc_ready--> load (loop) ; --doc_invalid--> resplit (loop) ; --questions_open--> ask-split ; --node_exhausted--> ask-tool ; --converged--> ask-task
ask-split(await ESC_INTENT_AMBIGUOUS, context:{questions:${@from.questions:-[]}})  --chose:answer--> resplit (loop) ; --chose:cancel--> cancelled
ask-tool(await ESC_TOOL_FAILURE)          --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
ask-task(await ESC_TASK_ESCALATED, context:{bugs:${@from.confirmed:-[]}})  --chose:retry_narrowed--> build (loop) ; --chose:split--> resplit ; --chose:override--> inventory ; --chose:cancel--> cancelled
integrate(child-workflow, workflow:epic-review-loop, base:$JOB_BASE, vars:{CONTEXT:${schedule.advisory:-[]}})
   --child_pass--> mark-epic ; --child_reviewed--> fix-integration ; --child_escalated--> ask-integration ; --child_cancelled--> cancelled
fix-integration(child-workflow, workflow:fix-loop-proved-clean, base:$JOB_BASE, goldens:[@from.confirmed, @from.failures, @from.context.bugs])
   --child_pass--> validate ; --child_reviewed--> ask-integration ; --child_escalated--> ask-integration ; --child_cancelled--> cancelled
validate(verify-tdd, mode:suite)          --implement_accepted--> integrate (loop) ; --implement_rejected--> fix-integration (loop) ; --converged--> ask-integration
ask-integration(await ESC_INTEGRATION_ESCALATED, context:{bugs:${@from.confirmed:-[]}, findings:${@from.failures:-[]}})  --chose:retry_narrowed--> fix-integration (loop) ; --chose:override--> validate (loop) ; --chose:cancel--> cancelled
mark-epic(record-marker, scope:epic-ready, child:integrate) --marker_recorded--> done[built] ; --matrix_uncovered--> ask-matrix
ask-matrix(await ESC_ACCEPTANCE_UNCOVERED) --chose:waive--> mark-epic (loop) ; --chose:cancel--> cancelled
convergence: any: [{max_iterations: 64}, {budget: {tokens: N}}]
```
(`ask-task` is reached only from `build`; integration-level escalations use `ask-integration`, whose
choices cannot rebuild or split an unrelated task. `schedule.advisory` accumulates children's advisory findings; `schedule` reads the job record's
`task-state` rows (and `resplit` writes the `split` row for the task it retires); the driver writes
`task-skipped {ids, deferredTo}` after `issue-skipped`'s output, one issue per skipped task id
(`taskset-load` emits `tasks_missing_credentials[]` beside the credential names); `build`'s `NOTE` is
the entering await's note (`retry_narrowed`) or the last checkpoint `adjust` note; `fix-integration`
receives `integrate.confirmed` on entry from `integrate` and `validate.failures` on entry from `validate`.
`all_tasks_done_integrate` = all done ∧ `$INTEGRATE == true` (standalone/fractal use); the `factory` passes
`INTEGRATE: false` because `finalize-loop` performs the integration review at the pushed head — so the
epic-ready marker is minted once per job, not twice.)

**`artifact-review-loop`** (family `artifact`, Phase 3; vars `RUBRIC`) — review only; what `review artifact`
inits: `discover(review-lenses, rubric:$RUBRIC) → adjudicate → done[clean|reviewed]`. It has no `await`,
so runner exhaustion is a driver `tool-park` on the parent (§5.3), as for bug-family children.

**`artifact-loop`** (family `artifact`, Phase 3; vars `TEMPLATE`, `RUBRIC`, `APPROVAL_CODE`,
`ITERATIONS_CODE`, `DOC_PATH`, `SOURCE_PATH?`):
```
author(author, template:$TEMPLATE, path:$DOC_PATH, source:$SOURCE_PATH, note:${ask-intent.note:-""})
   --doc_ready--> review ; --questions_open--> ask-intent ; --doc_invalid--> author (loop) ; --node_exhausted--> ask-tool ; --converged--> ask-iterations
ask-intent(await ESC_INTENT_AMBIGUOUS, context:{questions:${@from.questions:-[]}})  --chose:answer--> @from (loop) ; --chose:cancel--> cancelled
review(child-workflow, workflow:artifact-review-loop, base:${author.base}, vars:{RUBRIC:$RUBRIC})
   --child_pass--> approve ; --child_reviewed--> revise ; --child_escalated--> ask-iterations ; --child_cancelled--> cancelled
revise(author, mode:revise, path:$DOC_PATH, findings:${review.confirmed:-[]}, note:${@from.note:-""})
   --doc_ready--> review (loop) ; --doc_invalid--> revise (loop) ; --questions_open--> ask-intent ; --converged--> ask-iterations ; --node_exhausted--> ask-tool
approve                                   --approval_required--> ask-approve ; --approval_clear--> done[approved]
ask-approve(await $APPROVAL_CODE)         --chose:approve--> done[approved] ; --chose:revise--> revise (loop) ; --chose:cancel--> cancelled
ask-iterations(await $ITERATIONS_CODE)    --chose:override--> done[approved] ; --chose:revise--> revise (loop) ; --chose:cancel--> cancelled
ask-tool(await ESC_TOOL_FAILURE)          --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
convergence: any: [{max_iterations: 3}]
```
`doc_ready` = `doc_committed` ∧ `no_placeholders` ∧ ¬`questions_open` ∧ (`taskset_valid` when `TEMPLATE
∈ {decomposition, simple-task, split-task}`); `doc_invalid` = output present ∧ ¬`doc_ready` ∧
¬`questions_open` (uncommitted, placeholders, or an invalid taskset — every `author` output matches
exactly one of the three). `author.base` = the published head before
the step (a re-derived value; for a pre-existing `SOURCE_PATH` the review child diffs the whole document:
`base` = the parent of the commit that last touched it).

**`finalize-loop`** (family `shepherd`, Phase 2; vars `GOLDENS` default `[]`, `CONTEXT` default `""`;
`outputs: {confirmed: ${fix.confirmed:-[]}, fixed_golden_idx: ${fix.fixed_golden_idx@all:-[]},
golden_sources: ${fix.golden_sources:-[]}}` — the union over iterations, because a second `fix` after a
`validate` failure finds the first iteration's thread already fixed) — the shared "review at the pushed head, mint both markers,
run the gates" chain, entered after any driver-side or agent commit:
```
enter                                     --goldens_present--> fix ; --goldens_empty--> validate
fix(child-workflow, workflow:fix-loop-proved-clean, base:$JOB_BASE, goldens:[$GOLDENS, @from.confirmed, @from.findings, @from.failures, @from.context.bugs, @from.context.findings], vars:{CONTEXT:${@from.note:-$CONTEXT}})
   --child_pass--> validate ; --child_reviewed--> ask-final ; --child_escalated--> ask-final ; --child_cancelled--> cancelled
validate(verify-tdd, mode:suite)          --implement_accepted--> epic-check ; --implement_rejected--> fix (loop) ; --converged--> ask-final
epic-check(child-workflow, workflow:epic-review-loop, base:$JOB_BASE, vars:{CONTEXT:$CONTEXT})
   --child_pass--> pr-check ; --child_reviewed--> fix (loop) ; --child_escalated--> ask-final ; --child_cancelled--> cancelled ; --converged--> ask-final
pr-check(child-workflow, workflow:pr-review-loop, base:$JOB_BASE)
   --child_pass--> mark-epic ; --child_reviewed--> fix (loop) ; --child_escalated--> ask-final ; --child_cancelled--> cancelled ; --converged--> ask-final
mark-epic(record-marker, scope:epic-ready, child:epic-check) --marker_recorded--> mark-pr ; --matrix_uncovered--> ask-matrix
ask-matrix(await ESC_ACCEPTANCE_UNCOVERED) --chose:waive--> mark-epic (loop) ; --chose:cancel--> cancelled
mark-pr(record-marker, scope:pr-ready, child:pr-check)       --marker_recorded--> gate
gate(gate-review, scopes:[epic-ready, pr-ready], base:$JOB_BASE, evidence:${validate.receipts}) --gates_pass--> done[built] ; --gates_fail--> fix (loop) ; --converged--> ask-final
ask-final(await ESC_TASK_ESCALATED, context:{bugs:${@from.confirmed:-[]}, findings:${@from.findings:-[]}})  --chose:retry_narrowed--> fix (loop) ; --chose:split--> done[split] ; --chose:override--> validate (loop) ; --chose:cancel--> cancelled
convergence: any: [{max_iterations: 6}, {budget: {tokens: N}}]
```
(`enter` is node-less. `fix` is a merge point: `$GOLDENS` (the caller's findings — the shepherd passes
the triaged/CI findings) comes first so `fixed_golden_idx` maps back onto it; then the entering state's
channel — `epic-check`/`pr-check.confirmed`, `gate.findings`, `validate.failures`, `[]` from `enter` or
`ask-final` — and the `retry_narrowed` note replaces `CONTEXT`. The review-only `epic-check`/`pr-check`
children never commit, so both markers attest the head `fix`/`validate` left. `override`† records the
override grant as a disposition carrying `context.bugs ∪ context.findings` (§5.3 — a gate finding's
`id` is granted the same way) and routes through `validate` (so the head the leaf left is
suite/lint/typecheck-validated and `gate`'s `evidence:` always has a producer at this head) to a fresh
`epic-check`, which receives the dispositioned bugs as trailing goldens and passes because every
confirmed bug is dispositioned; `mark-epic`/`mark-pr` then mint `PASS_ADVISORY` markers carrying the
ids. `retry_narrowed` re-enters `fix` with the escalated bugs/findings as goldens via `@from.context.*`. **`gate`** runs the deterministic gate
commands `review epic-ready <epic> --base $JOB_BASE` and `review pr-ready --base $JOB_BASE` in the driver
worktree (the epic target located as `record-marker` locates the taskset) — the structural reviewers
(missing tests, `eval`, TODO, secrets, PR-evidence section) and the review log with `Covered paths:` that
the git-native pre-push hook and `status` read — **chaining `--previous-run` per scope while the
scope's latest job-wide gate run is non-pass**: `previous` defaults to the job record's `gate-run
{scope, run_id, head, verdict}` row (written by `gate-review` immediately after each gate command
returns, before the commit, with a `runs.jsonl` fallback for a crash in between), so a `NEEDS_REVISION`
log from an earlier iteration *or an earlier finalize child* is superseded rather than left to block
the hook (`${gate.run_ids}` covers only this run, and log supersession does not cross child runs); a
PASS row **ends** the chain, so the runchain's attempt counter counts consecutive failures only — the
CLI's cap is set above anything one finalize window can consume (`--max-attempts 64`), and the FSM's
own `gate --converged--> ask-final` (6) is the real bound, so the CLI never mints an `ESCALATED` log
inside a job (an `ESCALATED` log is a hard stop no later clean run supersedes, and the driver's own
pre-push hook would refuse every push after it); should a scope's row nevertheless read `ESCALATED`
(a pre-existing chain), `gate` reports `gates_fail` with that log as its finding and `ask-final` is the
exit. **It supplies the evidence the gate commands demand:** `review pr-ready` blocks on
`pr:missing-validation-evidence` without a successful validation receipt, and `review epic-ready`
blocks on `epic:missing-child-evidence` for every child without a PASS log or an evidence line — so
`gate-review` writes `<state-dir>/jobs/<job>/evidence.md` (receipts first, kept short: the readers cap
the file at 12 KB) from `validate.receipts` (the receipts minted at this HEAD, `sourceRevision = HEAD`;
a receipt with `exitCode ≠ 0` anywhere in the file flips pr-ready back to missing evidence, so only the
current head's are rendered) plus one line per **terminal `task-state` row of any state** — the literal
token `pass` on each (`\bpass\b` is what the reader matches; a marker verdict such as `PASS_ADVISORY`
is not it), citing the marker id for `built`, the grant ids for `overridden`, the issue URL for
`skipped`, the new ids for `split` — keyed by the **Beads child id** in Beads mode (`taskset-load`
records `{task_id, beads_id}` pairs in `created_ids[]`, and the driver creates the epic's Beads row at
`taskset-load` for an intent intake); nothing hand-written enters it. Then it commits the logs, which
touch only excluded paths and so leave both markers current (§5.3). Cost per finalize: one fix-capable review
(skipped when no goldens), one recheck if it fixed, two review-only passes, two gates.)

**`pr-shepherd-loop`** (family `shepherd`, Phase 4):
```
sync(sync-branch)                         --branch_synced--> enter ; --branch_foreign_protected--> ask-protected0 ; --branch_diverged--> ask-conflict
enter(marker-check, base:$JOB_BASE)       --markers_current--> push0 ; --markers_stale--> finalize0
finalize0(child-workflow, workflow:finalize-loop) --child_pass--> push0 ; --child_split--> ask-finalize ; --child_escalated--> ask-finalize ; --child_cancelled--> cancelled
push0(push)                               --pushed--> open ; --protected_paths--> ask-protected0 ; --push_rejected--> ask-push
ask-protected0(await ESC_PROTECTED_PATHS, context:{paths:${@from.protected_paths:-[]}}) --chose:allow--> @from (loop) ; --chose:cancel--> cancelled
open(open-pr)                             --pr_exists--> observe
observe(pr-observe)                       --pr_merged--> done[reviewed] ; --pr_closed--> ask-closed ; --ci_failing_ours--> finalize-ci ; --ci_failing_foreign--> ask-ci ; --conflicting--> merge-base
   --threads_actionable--> triage ; --threads_untrusted_only--> ask-untrusted ; --ready--> done[reviewed] ; --pending--> observe (loop) ; --converged--> ask-timeout
merge-base(merge-from-base)               --merged_clean--> finalize-ci ; --merge_conflicts--> resolve
resolve(resolve-conflicts)                --conflicts_resolved--> finalize-ci ; --conflicts_unresolved--> ask-conflict ; --node_exhausted--> ask-conflict
finalize-ci(child-workflow, workflow:finalize-loop, vars:{GOLDENS:${@from.failing_findings:-[]}})
   --child_pass--> push-ci ; --child_split--> ask-finalize ; --child_escalated--> ask-finalize ; --child_cancelled--> cancelled
push-ci(push)                             --pushed--> observe (loop) ; --protected_paths--> ask-protected ; --push_rejected--> ask-push ; --converged--> ask-timeout
triage(comment-triage)                    --fix_now--> finalize-tri ; --file_issue--> issue ; --reply_only--> reply ; --node_exhausted--> ask-tool
finalize-tri(child-workflow, workflow:finalize-loop, vars:{GOLDENS:${triage.findings}})
   --child_pass--> push ; --child_split--> ask-finalize ; --child_escalated--> ask-finalize ; --child_cancelled--> cancelled
push(push)                                --pushed--> reply ; --protected_paths--> ask-protected ; --push_rejected--> ask-push
ask-protected(await ESC_PROTECTED_PATHS, context:{paths:${@from.protected_paths:-[]}})  --chose:allow--> @from (loop) ; --chose:cancel--> cancelled
ask-push(await ESC_PUSH_REJECTED, context:{stderr_sha256:${@from.stderr_sha256:-""}})  --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
issue(file-issue, items:${triage.file_issue:-[]})  --node_output_present--> reply
reply(pr-reply, threads:${triage.replies:-[]}, filed:${issue.filed@iter:-[]})  --replied--> resolve-threads
resolve-threads(pr-resolve, goldens:${finalize-tri.goldens:-[]}, fixed:${finalize-tri.fixed_golden_idx:-[]}, sources:${finalize-tri.golden_sources:-[]}, replied:${reply.replied:-[]})
   --resolved--> observe (loop) ; --converged--> ask-timeout
ask-tool(await ESC_TOOL_FAILURE)          --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
ask-finalize(await ESC_FINALIZE_ESCALATED) --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
ask-ci(await ESC_CI_FOREIGN, context:{checks:${observe.checks:-[]}})  --chose:retry--> observe (loop) ; --chose:handled_externally--> observe (loop) ; --chose:cancel--> cancelled
ask-untrusted(await ESC_COMMENT_UNTRUSTED, context:{threads:${observe.threads:-[]}}) --chose:retry--> observe (loop) ; --chose:handled_externally--> observe (loop) ; --chose:cancel--> cancelled
ask-conflict(await ESC_MERGE_CONFLICT, context:{paths:${@from.conflicts:-[]}})  --chose:retry--> @from (loop) ; --chose:handled_externally--> sync (loop) ; --chose:cancel--> cancelled
ask-closed(await ESC_PR_CLOSED, context:{closed_by:${observe.closed_by:-""}})  --chose:retry--> open (loop) ; --chose:handled_externally--> cancelled ; --chose:cancel--> cancelled
ask-timeout(await ESC_SHEPHERD_TIMEOUT)   --chose:extend--> observe (loop) ; --chose:cancel--> cancelled
convergence: any: [{wall_clock: $SHEPHERD_TIMEOUT}]
```
(`sync` is the fork kind `sync-branch`: the driver fetches `origin/<job-branch>` and, when it is ahead
of HEAD, imports it with the §5.9 step-3 checks over `HEAD..FETCH_HEAD` — a maintainer's commit is
untrusted content — ff-merging it (`branch_synced`; unchanged or absent remote is also `branch_synced`),
parking on a protected path (`ask-protected0`), or, when the remote has diverged, on `ESC_MERGE_CONFLICT`
(`ask-conflict`; `handled_externally` after a human reconciles). A foreign commit therefore reaches
`enter` as a new HEAD → `markers_stale → finalize0`, which is how `release --child_failed--> ship
(loop)` recovers from `merge-check`'s `head_sha ≠ HEAD`. `enter` is the fork kind `marker-check` —
atoms are pure over `Snapshot`, so a state that must read `runs.jsonl` needs a node; it skips the
initial finalize when both markers are current at HEAD under the §5.3 rule — the factory's `finalize`
just ran. `observe`'s edges are listed in the atoms' evaluation
order (§5.10). Two finalize states keep the CI and triage channels apart: `finalize-ci` takes
`${@from.failing_findings}` (present on `observe`, absent on `merge-base`/`resolve` → `[]`),
`finalize-tri` takes `triage.findings`; **every triage route ends in `reply → resolve-threads`** (the
repo's own gtg practice: reply, then resolve): `pr-resolve` resolves the threads whose finding this
iteration's `fixed_golden_idx` maps (through `golden_sources`) onto the `$GOLDENS` segment, i.e.
`triage.findings[i].thread_id`, **plus every trusted thread that was just replied to** (`reply.replied`,
incl. issue-filed ones) — a declined nit with a justification must not leave a trusted thread open, or
`ready` is unreachable; `mixed`/untrusted threads are never resolved. `goldens`/`fixed`/`sources` all
come from **one** `finalize-tri` output (the child output carries the concatenated goldens list it
received), so a stale pairing is unrepresentable: on the `file_issue`/`reply_only` routes, or after a
push retry bumped the iteration, `pr-resolve` re-resolves threads an earlier iteration already resolved
— idempotent — and never mis-indexes. A `finalize-*` child that ends
`split`/`escalated` parks on `ESC_FINALIZE_ESCALATED` (no PR may
exist yet, so `ESC_CI_FOREIGN`'s observe-bound choices would be wrong there). Conflicts are merged from
`refs/metareview/base-tip`, never rebased. `pr_merged` handles a human merging in the UI. `push` runs with
`METAREVIEW_BASE` set to the merge-base it computes itself (§5.3) so the pre-push hook gates the attested range.)

**`release-loop`** (family `shepherd`, Phase 5a; vars `PR`):
```
merge-check(merge-check, pr:$PR)          --merge_conditions_met--> do-merge ; --approval_required--> ask-merge ; --conditions_pending--> merge-check (loop) ; --conditions_failed--> failed[failed] ; --converged--> ask-merge   # blocks on SHEPHERD_POLL like pr-observe; one iteration per state change
ask-merge(await ESC_MERGE_APPROVAL)       --chose:merge--> merge-check (loop) ; --chose:cancel--> cancelled
do-merge(merge)                           --merged--> ci-main ; --already_merged--> ci-main
ci-main(branch-observe, ref:$JOB_BASE_REF) --main_green--> runtime ; --main_pending--> ci-main (loop) ; --main_red--> ask-rollback ; --converged--> ask-rollback
ask-rollback(await ESC_ROLLBACK)          --chose:rollback--> rollback ; --chose:extend--> ci-main (loop) ; --chose:keep--> close
rollback(rollback, rollback_cmd:rollback) --rollback_ok--> reopen ; --rollback_failed--> ask-rollback (loop) ; --converged--> ask-rollback
reopen(task-close, epic:true, reopen:true)  --node_output_present--> learn
runtime(runtime-check, health_cmd:health) --runtime_verified--> close ; --runtime_not_required--> close ; --runtime_failed--> ask-rollback
close(close-issue)                        --node_output_present--> learn
learn(learn)                              --node_output_present--> done[released]
convergence: any: [{wall_clock: "2h"}]
```
`conditions_failed` = a non-transient condition is unmet (CHANGES_REQUESTED, a new untrusted thread,
`CONFLICTING`, a marker not current at HEAD) — the factory routes it back to the shepherd. `runtime-check`
receipts bind to `ci-main.head_sha`. After a successful rollback the job does **not** close the issue:
`reopen` (the one `task-close` call allowed after the merge, `reopen: true`) reopens the Beads epic with
the rollback note and records `rollback {sha, at}` in the job record, so the ledgers say "reverted",
not "shipped"; the leaf still ends `released` (the merge happened) and the row is what post-merge
learning reads. `learn` is a `{learned: false}` no-op until Phase 5b.

**`factory`** (family `factory`, Phase 6):
```
intake(intake)                            --size_simple--> simple ; --entry_design--> design ; --entry_decompose--> decompose ; --entry_build--> build ; --entry_ship--> finalize ; --entry_release--> release
design(child-workflow, workflow:artifact-loop, vars:{TEMPLATE:spec, RUBRIC:artifact, APPROVAL_CODE:ESC_SPEC_APPROVAL, ITERATIONS_CODE:ESC_DESIGN_ITERATIONS, DOC_PATH:${intake.spec_doc}, SOURCE_PATH:${intake.spec_path:-""}})
   --child_pass--> decompose ; --child_escalated--> ask-factory ; --child_cancelled--> cancelled
decompose(child-workflow, workflow:artifact-loop, vars:{TEMPLATE:decomposition, RUBRIC:plan, APPROVAL_CODE:ESC_PLAN_APPROVAL, ITERATIONS_CODE:ESC_PLAN_ITERATIONS, DOC_PATH:${intake.doc_path}, SOURCE_PATH:${intake.plan_path:-${design.path:-""}}})
   --child_pass--> build ; --child_escalated--> ask-factory ; --child_cancelled--> cancelled
simple(child-workflow, workflow:artifact-loop, vars:{TEMPLATE:simple-task, RUBRIC:plan, APPROVAL_CODE:ESC_PLAN_APPROVAL, ITERATIONS_CODE:ESC_PLAN_ITERATIONS, DOC_PATH:${intake.doc_path}})
   --child_pass--> build ; --child_escalated--> ask-factory ; --child_cancelled--> cancelled
build(child-workflow, workflow:epic-build-loop, vars:{SOURCE:${intake.source}, DOC_PATH:${intake.plan_path:-${intake.doc_path}}, INTEGRATE:false})
   --child_pass--> curate ; --child_escalated--> ask-factory ; --child_cancelled--> cancelled
curate(curate)                            --curation_human--> ask-learn ; --curation_auto--> curate-commit ; --node_exhausted--> ask-tool
ask-learn(await ESC_LEARNING_REVIEW)      --chose:accept_all--> curate-commit ; --chose:select--> curate-commit ; --chose:reject_all--> close-beads
curate-commit(curate-commit, ids:${ask-learn.note:-"*"}) --node_output_present--> close-beads
close-beads(task-close, epic:true)        --node_output_present--> beads-commit
beads-commit(beads-commit)                --node_output_present--> finalize
finalize(child-workflow, workflow:finalize-loop, vars:{CONTEXT:${build.advisory:-[]}}) --child_pass--> ship ; --child_split--> ask-factory ; --child_escalated--> ask-factory ; --child_cancelled--> cancelled
ship(child-workflow, workflow:pr-shepherd-loop)  --child_pass--> release ; --child_reviewed--> release ; --child_escalated--> ask-factory ; --child_cancelled--> cancelled
release(child-workflow, workflow:release-loop, vars:{PR:${intake.pr_number:-$JOB_PR}})
   --child_pass--> done[released] ; --child_failed--> ship (loop) ; --child_escalated--> ask-factory ; --child_cancelled--> cancelled ; --converged--> ask-factory
ask-factory(await ESC_JOB_ESCALATED)      --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
ask-tool(await ESC_TOOL_FAILURE)          --chose:retry--> @from (loop) ; --chose:cancel--> cancelled
convergence: any: [{max_iterations: 8}, {budget: {tokens: N}}]
```
`intake` emits `source` (`beads` when `.beads/` exists, else `plan_path` when given, else `doc_path`),
`doc_path`/`spec_doc` (from `docs.dir` and the job id), and the entry fields; `entry_ship` enters
`finalize` so both markers exist before the first push; the shepherd's bug-free `reviewed` leaf is
`child_pass` (every confirmed bug — there are none — is dispositioned), so `ship` routes both atoms to
`release`; `release` → `ship (loop)` re-shepherds after a non-transient merge condition. All driver-side
commits precede `finalize`, so the markers attest the pushed head. `close-beads` marks the epic closed
"pending factory job <id>" (the PR does not exist yet; `close-issue` annotates the number after merge;
§12.14 if `bd` cannot isolate the DB). Every `child-workflow` state without a `child_failed` edge —
including `build`, whose child declares explicit `failed[failed]` leaves — relies on the `child-park`
default (§5.3): its `retry` restores the tree to the child's checkpoint before re-driving, which a
`retry → @from` at `ask-factory` would not, and the `task-state` rows keep finished tasks finished.

---

## 7. Kinds and atoms (catalogue)

| Kind | exec | Family | Params | Output (`OutputFields`) | Attrs (MutatesTree / FixEntry / affinity) | Phase |
|---|---|---|---|---|---|---|
| `await` | host | shared | `code` (may be `$VAR`), `choices`, `context` (interpolated map) | `{choice, by, note, context}` (the interpolated map echoed, so `${<await>.context.<k>}` and `@from.context.<k>` resolve) | — | 0b |
| `child-workflow` | host | shared | `workflow`, `base` (default `$JOB_BASE`), `vars`, `goldens?` (source list: `<state>.<field>` \| `@from.<field>` \| `$VAR`, §5.3) | §5.3 verified output incl. `confirmed[]`, `path?`, `note?`, `fixed_golden_idx[]`, `golden_sources[]` | MutatesTree · driver | 0b |
| `record-marker` | fork | shared | `scope`, `child` | `{marker_id, head_sha, mode, verdict, dispositioned[], matrix?{covered, uncovered[], absent}}` | — · driver | 1 |
| `marker-check` | fork | shared | `base` | `{current, markers[{scope, head_sha, verdict}]}` — both markers current at HEAD under the §5.3 rule | — · driver | 2 |
| `sync-branch` | fork | shepherd | — | `{synced, imported_sha, protected_paths[], conflicts[]}` — fetches `origin/<job-branch>`; ahead → import under §5.9 step 3 (ff-only); diverged → `conflicts`; `branch_synced` / `branch_foreign_protected` / `branch_diverged` | MutatesTree · driver | 4 |
| `gate-review` | fork | shared | `scopes` (list), `base`, `evidence` (a map-valued field, passed as JSON: the `validate.receipts` it renders, with per-task pass lines from the job record, into the `--evidence` file) | `{pass, logs[], findings[{id, file, line, desc}], run_ids{scope}, evidence_sha256}` — runs the deterministic gate commands (`review epic-ready <epic>` / `review pr-ready`, `--base`, `--previous-run` chained from the job record's `gate-run` row while it is non-pass, `--max-attempts 64`; `epic-ready` skipped for taskset-less intakes) with the hardened env of §5.9, writes the `gate-run` rows, and commits their logs; `id` is the `findings.jsonl` id so an `override` at `ask-final` can grant it | MutatesTree · driver | 2 |
| `validate` (= `verify-tdd mode:suite`) | fork | shared | `cmds?` | receipts, `failures[{check, file, line, desc}]` | — · throwaway | 1 |
| `implement` | inline/subagent/fork(runner) | build | `context`, `max_node_attempts`, `brief_extra` | §5.8 | MutatesTree, FixEntry · clone | 1 |
| `verify-tdd` | fork | shared | `mode`, `cmds?` | `{receipts{red[{id, fail_before}], green[], suite, lint?, typecheck?, coverage?}, scope, secrets, failures[{check, file, line, desc}], fail_before}` | — · throwaway | 1 |
| `mutate` | fork | build | `mutate_cmd` | `{killed, survived, uncovered, unresolved, per_file[]}` | — · throwaway | 1 |
| `task-close` | fork | build/factory/shepherd | `epic?`, `reopen?` | `{id, prior_status, bd_closed, ledger_line, closed_ids[]}` | — · driver (DB only) | 1 · 5a (`reopen`) |
| `taskset-load` | fork | taskset | `source` | `{tasks[], baseline, credentials_missing[], tasks_missing_credentials[], protected_paths[], taskset_invalid, on_default_branch, created_ids[]}` | — · driver | 2 |
| `next-ready-task` | fork | taskset | `order` | union `{task_id, base, brief_path, checkpoint, advisory[]}` \| `{none, blocked_ids[], advisory[]}` | — · driver | 2 |
| `inventory-update` | inline/subagent/fork(runner) | taskset | — | `{path, appended}` | MutatesTree · clone | 2 |
| `author` | inline/subagent/fork(runner) | artifact | `template` (registry), `mode`, `path`, `source?`, `note?`, `task?`, `findings?` | `{path, commit, base, approaches[], questions[], invalid_reasons[]}` | MutatesTree · clone | 3 |
| `intake` | fork | factory | — | `{entry, size, source, doc_path, spec_doc, intent_sha256, issue, spec_path, plan_path, pr_number, trusted}` | — · driver | 3 |
| `file-issue`, `close-issue` | fork | shared / shepherd | `template?` (default `issue`), `items?` (list; one issue each) | `{filed[{thread_id?, number, url, body_sha256}], dry_run}` / `{number, url, dry_run}` | — · driver | 3 / 5a |
| `open-pr`, `pr-observe`, `branch-observe`, `pr-reply`, `pr-resolve`, `push`, `merge-from-base`, `merge-check`, `merge`, `runtime-check`, `rollback`, `beads-commit`, `learn` | fork | shepherd (`beads-commit` also `factory`) | fixed per §5.10; `ref`; `threads`+`filed?` (pr-reply: a filed issue's URL is appended to its thread's reply); `findings`+`fixed`+`sources`+`replied` (pr-resolve); `pr`; `health_cmd`; `rollback_cmd` | `PrState` (incl. `closed_by`) / `{ref, head_sha, checks[]}` / `{replied[], dry_run}` / `{resolved[], dry_run}` / `{pushed_sha, protected_paths[], stderr_sha256, dry_run}` / `{merged_clean, conflicts[]}` / `{conditions_met, approval_required, pending, failed, reasons[]}` / `{merged_sha, already_merged, dry_run}` / runtime receipt / `{exit, receipt}` / `{commit, changed}` / `{learned}` | push, merge-from-base, beads-commit: MutatesTree · driver; runtime-check, rollback: throwaway (egress) | 4–5a |
| `resolve-conflicts` | inline/subagent/fork(runner) | shepherd | — | `{resolved, commit}` | MutatesTree · clone | 4 |
| `comment-triage` | inline/subagent/fork(runner) | shepherd | — | `{findings[{thread_id, file, line, desc}], replies[{thread_id, body}], file_issue[{thread_id, title, body}]}` (trusted threads only) | — · clone (reads) | 4 |
| `curate` | inline/subagent/fork(runner) | factory | — | `{accepted[{fact, supersedes[]}], discarded[{reason}]}` | — · clone (reads) | 5b |
| `curate-commit` | fork | factory | `ids` | `{committed, rows}` | MutatesTree · driver | 5b |
| `integrate-branch` | fork | taskset | — | `{merge_sha}` | MutatesTree · driver | 7a |

| Atom | Predicate over `Snapshot` (absent output ⇒ false unless stated) | Phase |
|---|---|---|
| `node_output_present` | the state's node has an output this iteration | 0b |
| `chose:<name>` | latest `await` output in this state has `choice == name` | 0b |
| `converged`, `converged:<class>` | routed convergence stop for this loop-carrying non-await state (§5.3) | 0b |
| `node_exhausted` | `NodeAttempts ≥ max` or a non-retryable `LastProcessError` (true with absent output) | 0b |
| `child_pass` / `child_reviewed` / `child_built` / `child_split` / `child_failed` / `child_cancelled` / `child_escalated` | verified child output `outcome ∈ pass set` ∨ (`reviewed` ∧ every confirmed bug dispositioned — vacuously for a bug-free leaf) / `== reviewed` ∧ an undispositioned confirmed bug remains / `== built` / `== split` / `outcome ∈ {failed, stalled, custom} ∧ ¬escalated` (unrouted → `child-park`, §5.3) / `== cancelled` / `escalated` | 0b |
| `goldens_present` / `goldens_empty` | the JSON list in `Vars.GOLDENS` decodes to ≥ 1 element / to `[]` | 2 |
| `markers_current` / `markers_stale` | `marker-check` output `current` / ¬`current` (both markers current at HEAD, §5.3 rule) | 2 |
| `implement_done` / `implement_blocked` | `status ∈ {DONE, DONE_WITH_CONCERNS}` / `∈ {BLOCKED, NEEDS_CONTEXT}` | 1 |
| `tdd_verified`, `scope_respected`, `suite_green`, `lint_ok`, `typecheck_ok`, `coverage_ok`, `secrets_clean`, `implement_accepted`, `implement_rejected` | `verify-tdd` output (§5.8) | 1 |
| `confirmed_fixable`, `confirmed_unfixable_only` | bug fold: a confirmed bug is *unfixable* iff labelled `plan-mandated` or verdict ∈ the unverified set; `confirmed_fixable` = ≥ 1 fixable confirmed bug, `confirmed_unfixable_only` = confirmed non-empty ∧ none fixable | 1 |
| `plan_conflict`/`plan_clear`, `unverified_block`/`unverified_clear` | verified child output minus dispositioned bugs (§5.3 identity join) + `Policy` | 1 |
| `mutation_ok`, `mutation_survivors` (= output present ∧ ¬`mutation_ok`: survivors, uncovered sites, an incomplete score, or a source path with no mutant) | `mutate` output vs `Policy.mutation` | 1 |
| `marker_recorded`, `matrix_uncovered` | `record-marker` output | 1 / 2 |
| `load_ok` (= `taskset_valid` ∧ `baseline_green` ∧ ¬`on_default_branch` ∧ ¬`credentials_missing` ∧ ¬`protected_paths`), `taskset_invalid`, `baseline_red`, `credentials_missing`, `protected_paths`, `on_default_branch` | `taskset-load` / `push` output | 2 / 4 |
| `task_ready`, `all_tasks_done_integrate` (= all done ∧ `Vars.INTEGRATE == true`), `all_tasks_done_plain`, `deadlock`, `child_built_checkpoint` (= `child_built` ∧ `${schedule.checkpoint}`) | `next-ready-task` output; `Vars`; child output | 2 |
| `gates_pass` / `gates_fail` | `gate-review` output `pass` / ¬`pass` | 2 |
| `doc_ready`, `doc_invalid`, `questions_open`, `approval_required`/`approval_clear` (`Policy.human_approval[$RUBRIC]`) | `author` output; `Policy` | 3 |
| `size_simple`, `entry_*` (false when `size_simple`), `curation_human` (= `Policy.curation.review == human` ∧ `curate.accepted` non-empty), `curation_auto` (= output present ∧ ¬`curation_human`) | `intake`/`curate` outputs; `Policy` | 3 / 5b |
| `branch_synced`/`branch_foreign_protected`/`branch_diverged` (`sync-branch` output), `pushed`, `push_rejected`, `pr_exists`, `pr_merged`, `pr_closed`, `ci_failing_ours`/`_foreign`, `conflicting`, `threads_actionable`/`threads_untrusted_only`, `ready`, `pending`, `merged_clean`/`merge_conflicts`, `conflicts_resolved`/`conflicts_unresolved`, `replied`, `resolved`, `fix_now`/`file_issue`/`reply_only` | shepherd outputs (`dry_run == false` for writes); the `PrState` atoms are defined in evaluation order in §5.10, each excluding the earlier ones | 4 |
| `merge_conditions_met`, `approval_required`, `conditions_pending`, `conditions_failed`, `merged`, `already_merged`, `main_green`/`main_pending`/`main_red`, `runtime_verified`/`runtime_not_required`/`runtime_failed`, `rollback_ok`/`rollback_failed` (`exit == 0` / `≠ 0`) | `merge-check`/`merge`/`branch-observe`/`runtime-check`/`rollback` outputs | 5a |

Composite atoms are single names resolved by the family table; `∧` never appears in YAML.

---

## 8. Cross-cutting requirements

1. **Build process (unchanged):** TDD, DI via command seams (`BdRunner`, `GhRunner`, `cmdexec.Runner`,
   `GitRunner`, `Clock`, `Sandbox`, plus a driver `--stop-after <node>` hook), mock-AI, **100 % statement
   coverage on every package**, gremlins on new packages, gofmt/golangci-lint/shellcheck blocking. New
   packages: `internal/fsm/family`, `internal/worktree`, `internal/github`, `internal/factory`,
   `internal/adapters` (+ `internal/adapterstest`), `internal/scheduler`, `internal/coverage`,
   `internal/sandbox`, `internal/egress` (proxy + bridge), `internal/answers`. Table-driven negatives for
   every verified child field and every signature condition. `ssh-keygen` is a `tests/run-all.sh`
   prerequisite.
2. **Every exit criterion has two forms:** a **scripted** lights-out run under `tests/run-all.sh` (listed
   unguarded; a manifest test asserts every `tests/go/test-factory-*.sh` is listed) using committed sample
   repos `testdata/factory/{go (multi-package), vitest}/` with planted defects, fake adapters, a **fake
   OpenAI-compatible HTTP judge** keyed on request content with its `base_url` hashed into the job record,
   and a fake `gh`/`git push` remote; and a **smoke** run (`-tags smoke`) behind the sandbox. **The
   sandbox in scripted runs, stated once:** Phases 0–3 exits run `--unsafe-no-sandbox` (no GitHub write
   is on their path); Phase 4+ exits, which need `push`/`open-pr`/`merge` to produce real outputs against
   the fakes, run with the **`Sandbox` seam faked** — those suites build the binary with `-tags factorytest`, which
   swaps only the `Sandbox` and `Clock` implementations (the untagged binary has no fake path; a manifest
   test asserts the tag is used only by `test-factory-*.sh`); the self-test passes deterministically and
   the job record carries `sandboxed: true, fake_sandbox: true` (a value the smoke and platform jobs
   assert absent) — so `--unsafe-no-sandbox`'s production disable stays intact and is itself tested by
   per-kind negatives: under the flag and under `--dry-run`, each of `push`/`open-pr`/`merge`/`pr-reply`/
   `pr-resolve`/`file-issue`/`close-issue` outputs `dry_run: true` with a silent fake `gh`, and the
   shepherd parks on `child-park` at `push0`. Any Phase 0–3 exit with a write node on its path (the
   Phase 2 `skip_blocked`/`skip_tasks` issue filing) also runs under the faked seam and asserts one
   `create` on the fake `gh` per skipped id. Sandbox enforcement tests run in named platform jobs (`-tags sandbox_linux`
   with `bubblewrap` + userns sysctl; a macOS job) as positive/negative pairs incl. read-denial probes.
3. **Fail closed** at every new seam: unreadable taskset, unparseable adapter output, child mismatch,
   `gh` unavailable when a write was requested, empty allow-list, `ClsNoTest`, a policy without its cmd,
   an atom over an absent output, an untrusted intent, a secret in the diff, an unusable credential, an
   unsupported convention, a protected path at import.
4. **Embed + materialize** adapters, sandbox profiles, rubrics, templates, the egress bridge.
5. **Multi-language from Phase 1:** job-declared cmds + `test_convention` (`go|typescript|vitest`; `python`
   refused until 7a); zero-config detection in 7a.
6. **Security:** §5.12 in full; §14's extension points kept open.
7. **Docs updated in the phase that changes each shape:** `docs/ARCHITECTURE.md` §2–§4, §7;
   `docs/integrations/metaswarm.md` + `.integration.json`; `CLAUDE.md` *Durable Output*; `AGENTS.md`
   *Metaswarm Fit* (Phase 0b); `skills/*`, `commands/*`; `fsm --agent-prompt`; the memory note.
8. **Review-first, then bots** stays the recall yardstick for every PR this plan produces.

---

## 9. Phased plan

Eight epics across ten phase labels (0a/0b and 5a/5b are halves of one epic each; 7a/7b are tracks within
Phase 7). Exits are the two forms of §8.2 plus the named negatives.

### Phase 0a — Runners, routing, adapters, sandbox, egress, driver skeleton
- `runner:`; class-`_RUNNER` fallback under `factory run` (`mrv_exec_override`); generic runner executor;
  adapter contract; four adapters + `health`; taxonomy; `process_call`; `max_runner_timeout`; envelope
  builder moved to `machine`; the egress bridge.
- Class routing (implicit vars incl. `JOB_BASE` as a merge-base SHA, `SHEPHERD_*`, `CONTEXT`; `setup
  --install-adapters` prompts the operator per role class for model + runner with the shipped defaults
  (decision 12.5); `policy.answers: agentic` (decision 12.7);
  `models.yaml` with `provider`; env; precedence; floors; ladder; `allowed_providers`).
- `internal/sandbox` profiles + self-test (write and read probes); `internal/egress` per-invocation
  sockets, credential injection, per-request accounting; credential deny-list + `ERR_JOB_CREDENTIAL_UNUSABLE`;
  `--unsafe-no-sandbox` stamping; `METAREVIEW_STATE_DIR` + `state.Dir()`.
- Cmds channel (`--cmds-from`, `cmds_from_job` incl. `optional`, `egress:` with pinned files, declaration
  digest, runner cmds); consent by canonical name; `InitData.Consent`.
- `factory run/status/resume/export` skeleton driving any *existing* workflow unattended; job record
  (`ERR_JOB_EXISTS`, `{pid, start_time}`, the `allowed_signers` copy); driver parks (`budget-park`,
  `park-answer`); the three job verbs; `Clock`; driver-side git hardening (`GIT_CONFIG_GLOBAL=/dev/null`
  set inside `RealExec` and on every git-execing subprocess env, `--no-ext-diff --no-textconv`; the
  driver-owned `<state-dir>/gitconfig` for `push`/base-tip `fetch`; `Lstat`+`O_NOFOLLOW` for every
  driver-side worktree read/write).
- Export: registry-derived `knownKinds` with per-kind rules; children by reference; redaction reflection
  test extended.
- **Exit:** scripted: `factory run --workflow sdlc-loop-clean --base <ref>` on `testdata/factory/go` with a
  planted defect completes with ≥1 confirmed finding and a clean recheck, the fix node via the class
  `_RUNNER` fallback on two fake runners, zero `needs_input` envelopes surfaced; smoke: `claude-p` +
  `codex` through the bridge and proxy. Platform jobs: sandbox positive/negative pairs incl. a read probe,
  a proxy request without credentials in the sandbox, the bridge refused without its token, and loopback
  reachable inside the Linux netns. Also exercised: adapter fixtures per taxonomy code; empty stdout →
  retry; `budget-park` answered after `set-budget`; a floor violation; a ladder switch; a provider outside
  `allowed_providers`; an unusable credential; a `diff=` attribute no longer fatal; an HTTPS push
  authenticates through the driver-owned gitconfig with the user's global config absent; a driver write
  whose final component is a symlink is refused (`O_NOFOLLOW`) and one whose intermediate component is
  (`Lstat` walk), and likewise a driver *read* of `--evidence`/`.metareview-report/*`; a second `factory
  run` with the same `job_id` → `ERR_JOB_EXISTS`.

### Phase 0b — Engine: await, cancel, retry, routed convergence, composition, grammar, trees
- `await` (+ `$VAR` code, `context`, `@from`), `chose:*`, `PARKED`, `factory answer` (+ `internal/answers`;
  `paths[]` in the payload), `defer` verb, outcome classes + `runs.jsonl` verdicts/statuses, explicit `failed`
  edges, implicit `cancelled`, `fsm cancel`, `factory cancel [--discard]` (incl. reopening closed Beads
  rows, `refsrc`/discarded cleanup) and `factory gc`, `status` "parked".
- Retryable class (+ `import_diverged`), `WarnData` fields, `NodeAttempts`/`LastProcessError`, restore
  protocol with discarded refs, no re-execution when exhausted, `mrv_exec_override`, reserved record names
  + `Snapshot.Records`, `tool-park`, `child-park`, `import-park`.
- `child-workflow` (driver-level), `InvokedBy` (copied by `Fork`; lineage-aware `DecodeIn`), `child-spawn`
  (+ `golden_sources`), init-field verification and the output fields (`confirmed`, `unverified`,
  `plan_mandated`, `advisory`, `path`, `note`, `fixed_golden_idx`, `golden_sources`), the non-bug
  `outputs:` map, the goldens grammar (`<state>.<field>` | `@from.<field>` | `$VAR`, concatenation,
  `Golden.SourceID`, dispositioned bugs as trailing goldens), child retry rule (checkpoint definition;
  deterministic failures and `overflow` not retried), parked passthrough, escalated-findings projection +
  `task-overridden`/`deferred` dispositions, `disposition` records with `bugs[]` and the identity join,
  child→parent `VerifyOrigin`.
- Multi-loop grammar, routed `converged` with precedence, `AnswerWindow`, `loop_source_without_converged`
  + `cycle_without_loop`, the three unrouted parse rules + `child_failed`'s park default,
  `node_output_present`, latest-at-or-before interpolation with nested defaults, dotted map fields and the
  path class, `@from` (node-less → default), `await.context` echo, `--vars-file`, `OutputFields`,
  recursive map params, `$VAR` in convergence, `--policy` fold, `wall_clock`, `MutatesTree`/`FixEntry` (+
  `ToFixEntry`, `InitialFixEntry`, `fix_baseline`), tree affinity, fork-exec tree re-read, `cmd_params`,
  `Finding.Labels`/`Bug.Labels`, the **marker-currency rule** in `LatestReviewEvidence`/
  `validateFromRunDiff`/`record-lenses`/the hook.
- `internal/worktree` (agent clone, driver worktree, ref source with `refs/metareview/base-tip`,
  publish/mirror(destination refspecs, `checkout -B` from `refs/metareview/mirror-head`)/import(sandboxed
  upload-pack, protected-path + symlink check with the base-tip blob exemption, fsck, ff-only)/restore/
  throwaway protocol (`clone --shared` + `checkout --detach`; `Reproducer` materialiser seam),
  registration + clean-Beads checks), two-root rule, split-state detection + terminal-only `state
  migrate`, `--auto-worktree`.
- Docs: coexistence rule (§13) in the five documents.
- **Exit:** golden replays of **pre-refactor recorded audits** for all five shipped workflows (recorded from
  the `c0b933b` binary, incl. a `prove` run) fold to identical canonical snapshots; a scripted
  parent→child run parks on an `await`, is answered (unprivileged) and resumes without a fork; a privileged
  answer is refused unsigned and accepted signed. Negatives: child ends `failed` at attempt 1 → driver
  retry-forks → third failure → `child_escalated` and its findings appear in `findings.jsonl`; an
  `overflow` child escalates without a retry; a deterministic-failure child at a state with no
  `child_failed` edge parks the parent on `child-park` and `retry` re-drives it from its checkpoint; an
  `explicit_fail` child routes `child_failed`; wrong `base`/`work_dir`/`mock`/workflow hash →
  `ERR_CHILD_MISMATCH` (one test per field); a non-bug child whose `outputs:` map does not reproduce its
  leaf's `NodeOutputs` → `ERR_CHILD_MISMATCH`; tampered child audit; torn child → repair-resume; a
  dispositioned bug re-supplied as a golden keeps its `SourceID` and counts as dispositioned in the
  re-review; a retry-forked parent re-verifies a
  copied child output; `--stop-after` kill and resume; `factory cancel --discard` disposes per the table;
  `fsm cancel` on a running run with an in-flight fake adapter; a retry warn in a previous iteration does
  not count; `WarnUnsanctionedEdit` does not count; a `converged` state whose answer re-arms the window
  proceeds on the next traversal and an accepted forward edge is not shadowed by `converged`; the mirror
  resets a diverged clone (branch and tree); the import refuses a non-fast-forward, parks the refused head,
  and counts `import_diverged`; the import refuses an agent-committed `.gitattributes`, a nested
  `sub/.gitattributes`, an agent-committed symlink under `docs/metareview/` **and one under `src/`**
  (an added symlink, and a regular file type-changed to one whose blob equals base-tip's), and a
  `.beads/` edit before checkout, parks on `import-park`, and a signed `allow` re-runs it (the adapter
  was called once; the spooled output is recorded); a protected path whose blob equals
  `refs/metareview/base-tip`'s is imported without an allowance, while the same path modified on top by
  the agent (blob ≠ base-tip's) is refused, and an agent that resolves `.beads/issues.jsonl` or the
  knowledge table by taking theirs (blob == base-tip's ≠ the driver head's) is refused; a throwaway tree
  is materialised from a read-only `refsrc`, `cmds.install` populates it from a seeded job cache (the
  vitest fixture ships a committed cache tarball; no network), the Go install runs once for two
  consecutive throwaways with an unchanged `go.sum` and again after a task changes it (call count 1 → 2)
  while the vitest install runs in every throwaway, and the `Reproducer` runs in it; the mirror creates `refs/metareview/base-tip` in the clone and resets it after a backwards restore
  (forced refspec); a discarded head stays reachable; `status` reports a driver park as parked; `state
  migrate` moves a terminal run and refuses a parked one; `--auto-worktree` creates the checkpoint tree;
  a note crosses a loop edge to its consumer; a `retry` returns to `@from`; an `@from` from a node-less
  state takes its default; `@all` unions two iterations and `@iter` defaults outside the current one; a
  `stalled` deterministic leaf parks on `child-park`; a marker whose `headSha..HEAD` diff touches only
  `docs/metareview/**` is current and one touching a source file is stale — one pair per reader:
  `LatestReviewEvidence`, `validateFromRunDiff`, `record-lenses` (refuses), `record-marker`
  (`ERR_MARKER_STALE`); one rejecting fixture per parse rule (`loop_source_without_converged`,
  `cycle_without_loop`, `child_outcome_unrouted`, `runner_exhaustion_unrouted`, `await_choice_unrouted`)
  plus an await-only cycle that is accepted, and a fixture that parses every shipped and §6 workflow
  (with stubbed `KindInfo`/atom tables at 0b, re-run against the real registry by a per-phase manifest
  test as each kind lands); a non-driver reserved record is refused; a second
  `park-answer` for one `esc_id` is refused; the fixture workflows for these negatives live under
  `testdata/fsm/factory/`.

### Phase 1 — The fractal unit: `task-build-loop`
- `implement` (+ `brief_extra`), `verify-tdd` (differential RED rule; per-package counters;
  `internal/coverage`; production-source predicate; secrets scan with raw tails; throwaway trees in their own
  sandbox; `failures[]`), `mutate` (per-source-file rule; `no-tests` waiver), `record-marker` (currency
  rule incl. dispositioned children, `verdict`, refusals), `task-close` (`prior_status`; no reopen after
  merge except the sanctioned `reopen: true` rollback path), `validate`; `fix-loop-proved-clean` with `confirmed_fixable` and `CONTEXT`; `review-lenses`
  `context:` param; the `plan-mandated` label; `await` `context:` maps on the three task awaits; the
  prompt validator; the implementer prompt (Superpowers 5-part recipe + metaswarm Coder rules +
  `systematic-debugging`; "uncommitted work is discarded"); `triage` → goldens; `plan_conflict`/
  `unverified_*` policy paths with persistent dispositions; `ask-tool` routing.
- `tasksource` structured fields (#2 **D1**); `BdRunner` (verified verbs, auto-flush off, DB isolation —
  decision §12.14 if impossible); `Metareview-Run:` trailer; surviving-mutant severity honours policy;
  receipts gain `sourceRevision` + reader rule.
- **Absorbs:** #2 D1, D3, D5 (file:line + `plan-mandated`).
- **Exit:** the multi-package Go and the vitest sample repos each build one task from a plan block
  (scripted, fake HTTP judge) and the Go one from Beads (smoke, `bd` pinned): a planted defect yields ≥1
  confirmed finding fixed by the child, a `subagent-adjudicated` task-done marker at the final head,
  `mutation_ok`, and `review task-done` PASS with no hand-run `record-lenses`. Negatives: forged receipts
  ignored; a greenfield test (compile failure at base) passes RED; a pre-existing failing test at bare base
  does not; an unchanged same-package test listed in `tests_added` fails RED; a no-assert `ClsCompile` test
  under `mutation: required` fails `mutation_ok`; `[no test files]` in the task's package fails
  `suite_green` even with other packages green; out-of-scope file → `scope_violated`; a task touching only
  `go.mod` + source passes per-file rules on the source file only; a `no-tests` task touching source passes
  `mutation_ok`; a `mutate` cmd covering none of the task's files fails; a coverage report missing a task
  source file fails; policy without cmd → `ERR_JOB_POLICY_UNBACKED`; mock child → `ERR_MARKER_MOCK`; retry
  after `ERR_PROCESS_TIMEOUT` starts from the entry head with partial commits discarded; an unverified
  finding parks under `block`, continues under `advisory` with a `PASS_ADVISORY` marker, and a `dismiss`
  answer yields a marker carrying the dispositioned id that does not re-park on the next iteration;
  `treat_as_real` and `fix_code` re-enter `review` with a `child-spawn.golden_sources` row whose
  `@from.context.*` segment is non-empty and whose `SourceID`s equal the parked bugs' ids, and after
  `fix_code` the plan-mandated bug is `confirmed_fixable` and fixed; a `treat_as_real` bug the child
  leaves unfixed re-parks on `ask-unverified` and never mints a marker (`fixable` is directive, not
  covering); a plan-mandated finding parks on `ESC_PLAN_CONFLICT`; a credential value in a test fixture or in test
  stdout fails `secrets_clean`; a 1-byte credential is refused at job start; `ESC_TASK_BLOCKED` after the
  ladder, and its `context` note reaches the next brief.

### Phase 2 — `epic-build-loop`, `finalize-loop`, `pr-review-loop`
- `taskset-load` (Beads per iteration incl. idempotent `bd create`; plan block; `taskset_valid` with the
  split convention; baseline; credentials; protected paths; `taskset_invalid`), `next-ready-task` (plan
  order; `task-skipped`/`task-overridden` records; advisory accumulation; `allowed_paths`),
  `inventory-update`, checkpoints, `resplit → load` with `task`/`note`, `issue-skipped`, `ask-task`,
  `ask-tool` with `@from`; `epic-review-loop` as child with `CONTEXT`; `fix-integration` + `validate`;
  `record-marker --scope epic-ready` with the **acceptance coverage matrix** (#2 **D2**; `waived |
  accepted-risk | deferred:<url>` dispositions, #2 **D5/D6**; `ESC_ACCEPTANCE_UNCOVERED`; the taskset
  located via the job record, `matrix.absent` for taskset-less intakes); shard results enter
  `findings.jsonl`; **`review-lenses` shard mode** (§5.3); `finalize-loop` (with `enter`, `gate-review`
  chaining `--previous-run`, the merge-point goldens, override → re-review, `outputs:`) and
  `pr-review-loop`; `marker-check`; durable `task-state` job rows incl. `split`; `INTEGRATE`, `DOC_PATH`,
  `NOTE` vars; `answer-relay`; `cmds.install`.
- **Exit:** a 4-task epic with a dependency chain builds lights-out from a plan block (both sample repos;
  the vitest one through `cmds.install`); a planted cross-task contradiction is caught by the integration
  lenses and fixed, then `validate` passes; the finalize marker's matrix has ≥ 1 cell and none uncovered;
  a defect planted so it is visible only over `$JOB_BASE..HEAD` is fixed by `finalize-loop.fix` and both
  markers are **current at HEAD** after `gate` commits its logs (`headSha` = the head `fix` left ≠ the
  entry head, `headSha..HEAD` touching only `docs/metareview/**`); a second `pr-shepherd-loop.enter` on
  that head reports `markers_current`; the `gate` node's logs exist, its second iteration chains
  `--previous-run` so the first `NEEDS_REVISION` log no longer blocks, a scripted seventh consecutive
  failing gate still does not mint an `ESCALATED` log (a PASS row ended the chain), `review pr-ready`
  passes on the evidence file `gate-review` rendered with no hand-supplied `--evidence` (unit fixture:
  evidence rendered from `task-state` rows of every state incl. one `built` under a `PASS_ADVISORY`
  marker, one `overridden`, one `skipped`, one `split` → `missingChildEvidence == []`; a `built` row
  removed → its id reported; and in the Beads smoke `review epic-ready` passes `missing-child-evidence`
  from those lines), the `override` at `ask-final` re-validates so `evidence_sha256` derives from
  receipts at HEAD, an `override` at `epic-build-loop.ask-task` under `factory.build` reaches
  `finalize.epic-check` as a golden and `mark-epic` mints `PASS_ADVISORY` carrying that id, and the
  pre-push gate accepts the branch with `METAREVIEW_BASE` — while a push with an unsuperseded
  `NEEDS_REVISION` log on the branch is refused by the hook through the driver gitconfig
  (`push_rejected`, `stderr_sha256`) and succeeds once `gate-review` chains it; a diff larger than one context is reviewed in shard mode
  with `[shard-xx]` findings merged while a tree-resident shard result is ignored; a diff over
  `policy.review.max_diff_bytes` fails with `ERR_JOB_DIFF_TOO_LARGE`; a task escalated once and rebuilt
  via `retry_narrowed` has its projected findings closed and passes the epic gate; a `resplit` with
  placeholders loops and one with an invalid taskset is refused by `doc_ready`; a `plan_path` job's
  `resplit` writes to the plan document `load` reads.
  Negatives: T2 `split` with nothing else ready → `ESC_DEP_DEADLOCK` → `skip_blocked` files an issue and
  resumes; `checkpoint: true` parks and resumes with an `adjust` note in the next brief; overlapping `files`
  fails `taskset_valid`; `ESC_CREDENTIALS_REQUIRED`, `ESC_BASELINE_RED`, `ESC_PROTECTED_PATHS` (with the
  allowance reaching the child) park and resume; an integration fix that breaks lint fails `validate` and
  loops; `override` at `ask-task` records `task-overridden` and the scheduler does not re-schedule it;
  `resplit` re-loads with appended ids and Beads rows created; an uncovered matrix cell parks and `waive`
  mints the marker; a `retry_narrowed` at `ask-task` re-runs `build` with the same task and a note; a
  retry-forked `epic-build-loop` does not re-schedule a task the job record marks built; an `override` at
  `ask-final` re-reviews (the overridden bugs arrive as trailing goldens and match by `SourceID`) and
  mints `PASS_ADVISORY` markers carrying the ids; a `no-tests` label on a task with production source
  fails `taskset_valid` without plan approval; `gates_fail` (a planted TODO) loops to `fix` **with the
  gate finding as a golden** and converges to `done[built]` on the next gate; a `validate` failure
  re-enters `fix` with `failures[]` as goldens; a persistent unfixable gate finding trips `gate`'s
  `converged` edge to `ask-final`; a split task's retired id is never re-scheduled; a child answer
  re-arms the parent's window (`answer-relay`); the three `AnswerWindow` counters each re-arm;
  `mutate`/`coverage` in the sample repos run pinned gremlins/Stryker (listed as `tests/run-all.sh`
  prerequisites), so the mutation negatives are not canned.

### Phase 3 — Front end: intake, design, spec, decomposition
- `artifact-review-loop` (what `review artifact` inits; idempotent on content hash; resumes; structured
  ingestion; `Rubric:` header honoured only when the artifact's content class matches — #2 **E1–E6**) and
  `artifact-loop` (`DOC_PATH`, `SOURCE_PATH`, `revise` with `findings`/`note`, `doc_invalid` loop).
- `author` templates **spec** (approaches[], questions[]), **decomposition**, **simple-task**,
  **split-task**, **issue** — embedded registry; `rubrics/plan-review-rubric.md` embedded; `intake`
  (entries incl. `spec_path`/`plan_path`/`pr_number`, size, `doc_path`/`spec_doc`, trust,
  `ERR_INTAKE_UNTRUSTED`); `file-issue`.
- **Absorbs:** #2 E (superseded in shape).
- **Exit:** from a one-paragraph intent the factory produces a committed spec and a decomposition at the
  intake-named paths passing `taskset_valid` and the lens gate with at most one park; an existing
  hand-written spec enters at `spec_path` and is reviewed, not rewritten (review base = its last-touch
  parent); after `revise` → PASS the job record holds a `review-run` row per document, the earlier
  `NEEDS_REVISION` log is superseded, and `status`/the push gate report no blocker for it; a legacy
  hand-edited `NOT_REVIEWED` log still parses and blocks; three failed iterations park on
  `ESC_PLAN_ITERATIONS` and `revise` re-arms the window and carries the note; a plan-rubric header on a
  spec log is not honoured; `bd create` write-through is observed (smoke); a `factory:filed` issue is
  refused as intent.

### Phase 4 — Shipping: `pr-shepherd-loop`
- `internal/github`: GraphQL threads/latest-approvals/protection/`pr_state`, checks + "ours" log tails →
  `failing_findings`, gtg (pinned); blocking `pr-observe`/`branch-observe`; writes with fixed argv, object
  scope, consent, `dry_run`, three-dot protected-path check; per-comment trust; `PrState`; `--github-pr` in
  `review pr-ready` switches to threads (#2 **F1–F6**, incl. `external_review: required` and a
  content-anchor final-tree check).
- `comment-triage` (findings output; trusted threads; metaswarm's matrix) + `receiving-code-review`
  discipline; `merge-from-base` (abort-on-conflict, base published to refsrc) + `resolve-conflicts`
  (clone-side merge of `refs/metareview/base-tip` after the driver's `.beads` resolution; `.beads/**`
  denied); `finalize` before every push with `GOLDENS` on split CI/triage routes; `pr-resolve` mapping
  `fixed_golden_idx` through `golden_sources` → thread ids on the fix route only; the ordered `PrState`
  atoms incl. `ready`; `push_rejected`/`ask-push`; `ask-finalize`; `marker-check`; `ask-protected0`;
  `ESC_PR_CLOSED` with `closed_by`; blocking observe with `SHEPHERD_OBSERVE_MAX` → `pending`; `wall_clock`
  + `AnswerWindow`; `file-issue {items}`.
- **Absorbs:** #2 F; memory *metareview-pr-shepherding-and-status-gap* quirks.
- **Exit:** scripted with a fake `gh` (a state machine the script drives; every transition below is
  asserted in order): a PR with thread **T1** (CodeRabbit, trusted, a real defect), **T2** (CodeRabbit
  with a stranger reply — `mixed`), **T3** (stranger-opened), a failing "ours" check, and a conflicting
  base takes `ci_failing_ours → finalize-ci → push-ci`, then `conflicting → merge-base → finalize-ci`
  (both markers refreshed before each push), then `threads_actionable → triage(fix_now) → finalize-tri`
  (T1's finding a golden of the fix child) `→ push → reply → resolve-threads` resolving **T1 only**, then
  then the fake opens a second trusted thread **T4**, triaged `reply_only`, replied to *and resolved*
  (`resolve-threads.resolved` is exactly `[T1]` then `[T4]` per iteration), then
  `threads_untrusted_only → ask-untrusted` where a signed `handled_externally` (the fake marks T2/T3
  resolved by a human) leads to `ready`; T2 is never auto-resolved; in a separate scenario a T1 fix that
  breaks lint re-enters `fix` via `validate` inside the same `finalize-tri` and T1 is still resolved
  (`@all`), and a push rejected then retried still resolves T1 (indices paired with the same child
  output); the
  4 h window trips `ESC_SHEPHERD_TIMEOUT` and `extend` opens a new window — tested with both an
  oscillating fake `gh` and a frozen one (the observe deadline emits `pending`); `factory cancel` while
  `observe` blocks on the frozen fake kills the subprocess and cancels, and a job row whose
  `start_time` mismatches is not signalled; gtg installed with `status ≠ READY` on a green, thread-free
  PR keeps `observe` on `pending`; the `file_issue` path files one issue per item and replies with the
  URLs; a human merge in the UI ends the loop `reviewed`; a PR the factory closed parks on
  `ESC_PR_CLOSED` and an unsigned `retry` reopens it, one a maintainer closed refuses the unsigned
  `retry`, `handled_externally` ends `cancelled`; the initial `enter` skips `finalize0` when the factory's
  markers are current; with gtg not installed a green, thread-free PR is `ready`; a remote-refused push
  parks on `ESC_PUSH_REJECTED` and `retry` pushes; a `finalize0` escalation parks on
  `ESC_FINALIZE_ESCALATED` before any PR exists; a CI check that keeps failing "ours" trips `push-ci`'s
  `converged` edge to `ask-timeout`; a source commit pushed to the job branch by a maintainer makes
  `enter` report `markers_stale → finalize0` and `merge-check` `conditions_failed`; a retried
  `finalize-ci` child's first `NEEDS_REVISION` gate log is superseded through the `gate-run` row and the
  hook accepts the push; a `.beads`-conflicting base is merged with the job's closes intact; smoke on a
  sandbox GitHub repo. Negatives: a comment instructing
  `git push --force` produces no push (platform job); `push` to `main` refused; a first push touching
  `.github/workflows` parks on `ask-protected0`, a signed `allow` pushes and opens the PR; a merge from
  base that touched `.github/**` does not trip the three-dot check nor the import check (blob equals
  `base-tip`); an agent-committed `.gitattributes` is refused at import.

### Phase 5a — Release and close: `release-loop`
- `merge-check`/`merge` (external condition bound to `head_sha`; latest review per login; protection only
  for `auto`; `conditions_pending`/`conditions_failed`; the signed answer consumed as a condition;
  `already_merged`), `branch-observe`, `runtime-check` bound to `main` (#2 **G1–G4**) + the **gate-side
  claim detector** for non-factory gates (`--runtime-evidence`, claim scanner), optional `rollback`
  (`egress:` cmd at `$JOB_BASE`; the answer refused when undeclared) and `extend`; `close-issue`;
  `task-close --epic` (rides the PR) + `beads-commit`; `factory.release --child_failed--> ship (loop)`;
  `JOB_PR`; the `learn` no-op stub (real in 5b).
- **Absorbs:** #2 G.
- **Exit:** scripted merge with a fake latest approval bound to head; refusal when the approval predates a
  push or is followed by CHANGES_REQUESTED; refusal unsandboxed; `merge: auto` refused on an unprotected
  repo at job start, `merge: human` with a signed answer merges on one and the answer is consumed by
  `merge-check`; a non-transient condition routes back to the shepherd; `main_red` → `ESC_ROLLBACK` →
  `rollback` cmd through the proxy, a non-zero rollback re-parks, `extend` re-observes main, `keep`
  closes, and `rollback` is refused when `cmds.rollback` is undeclared; an `allowed_signers` file edited
  after job creation does not change what verifies (the copy does); `policy.finish: pr` removes the
  job's trees after `done[released]` and `keep` leaves them; a `task-close` without `reopen: true` after the merge refuses to
  reopen (the sanctioned `reopen: true` rollback path is the one exception); a failed health check blocks; a non-factory `review
  pr-ready` on a PR whose body claims "deployed" blocks without a runtime receipt; the Beads epic close is
  in the merged diff.

### Phase 5b — Knowledge and handoff
- One knowledge table (`knowledge.Path`, dual-root `Collect`), `curate` (proposes; non-bot trusted
  sources; state-dir proposals folded into its envelope by the driver) + `curate-commit` (`ids` = the
  JSON id list in the answer note; `supersedes` rules; default `auto`), post-merge `learn` to the state
  dir, repo-scoped `sessionhistory`, priming (fenced); `factory export` → `HANDOFF.md` (children by
  reference); `factory resume`; #2 A6; #2 DoD replay of two historical sessions, false-gate count recorded
  in `docs/metareview/learning/`.
- **Exit:** a killed-and-resumed job continues from the same node; accepted learnings from a previous job's
  post-merge `learn` reach the next job's implementer payload as fenced data and ride its PR; a stranger
  comment and a bot-relayed stranger comment cannot become accepted rows under `auto`; a `select` answer
  commits only the listed ids; a cross-job `supersedes` is refused under `auto`; existing
  `.beads/knowledge/` rows are still primed; a `factory export` of a nested job fits the per-run cap.

### Phase 6 — The `factory` workflow
- `intake` routing (`size_simple` first; entries; untrusted refusal; `doc_path`/`spec_doc`), the end-to-end
  graph incl. `close-beads → beads-commit → finalize → ship → release`, `ESC_JOB_ESCALATED` with `@from`,
  `factory status` with cost (prices + proxy accounting), `factory escalations`, `ESC_CONSENT_REQUIRED`
  with `accept-consent`.
- **Exit (dogfood):** the factory builds the Phase-7a python-sample epic in metareview itself end to end
  behind the sandbox, within budget, parking only on `ESC_MERGE_APPROVAL` (and `ESC_PROTECTED_PATHS` if a
  workflow file is touched); scripted end-to-end on `testdata/factory/go` **reaching `release` and
  `done[released]`** through `ship`'s `child_pass` edge (with the faked `Sandbox` seam of §8.2),
  including the simple path, a clean `entry_ship` PR whose markers come from `finalize` alone and whose
  epic-ready marker records `matrix.absent: true` with verdict `PASS_ADVISORY`, a consent park answered
  with `accept-consent`, a deterministic child failure at `factory.design` parking on `child-park` rather
  than failing the job, a second job consuming the first's post-merge proposals exactly once (the
  `consumed` marker), and — under `--unsafe-no-sandbox` — the same job parking on `child-park` at `push0`
  with every write node's output `dry_run: true`.

### Phase 7 — Scale and robustness (7a) and hygiene (7b)
- **7a:** per-task clones with disjoint `files`, `integrate-branch`, bounded waves, `fix-fanout` by
  package; zero-config test detection; python convention + sample repo + pytest parsers;
  `mutation-hardening` workflow. **Exit:** the python sample repo builds a 6-task epic with two parallel
  waves, lights-out.
- **7b:** session-start self-heal; `setup --check` factory prerequisites; optional `visual-review` node;
  `coveredPaths` writer; `docs/ARCHITECTURE.md` rewritten around families; the `sdlc-loop*` → `fix-loop*`
  rename with aliases (retired one release later). **Exit:** each item's scripted test.

---

## 10. Issue #2 reconciliation

| #2 item | Status today | Disposition |
|---|---|---|
| A1–A5, B1–B6, C1–C8 | done | — (B gains optional `sourceRevision`, Phase 1) |
| A6 | partial | Phase 5b |
| D1 | not done | Phase 1 |
| D2 | not done | Phase 2 (`record-marker --scope epic-ready` computes the matrix) |
| D3 | not done | Phase 1 (factory) / §13 (outside) |
| D4 | done | — |
| D5 | partial | Phase 1 (`plan-mandated`, file:line) + Phase 2 (acceptance) |
| D6 | partial | Phase 2 |
| E1–E6 | not done (E4/E6 partial) | **Superseded in shape** by the `artifact` family (Phase 3) |
| F1–F6 | not done (F4/F6 partial) | Phase 4 (F5 content-anchor check included) |
| G1–G4 | not done (G2 partial) | Phase 5a (incl. the gate-side claim detector) |
| DoD replay of two historical sessions | not done | Phase 5b |
| Review Manifest / Projector / Sharded FSM (#2 §1–3) | partial/done | matrix + job projection; projector done; shard results into `findings.jsonl` in Phase 2 |

---

## 11. What we deliberately do not replicate

- **Prompt-only enforcement**; replaced by gate atoms, rubric text reused as node prompts.
- **Persona sprawl** (19 agents): classes + rubrics; PM/Designer/Security-design/CTO are lenses;
  Researcher is a step of `author`; ops personas out of scope.
- **Team Mode, `bd swarm *`, Slack daemon, weekly reports** — aspirational or absent in metaswarm.
- **`TodoWrite` ban / Beads-only todo policy** — the run store is the ledger.
- **Interactive (Socratic) brainstorming and its visual companion** — the host's skill, outside the factory;
  the factory enters at `spec_path`/`decompose` (decision §12.9, not yet accepted).
- **`/migrate`, `/update`** — another product's packaging (§13 covers ours).
- **A deployment pipeline** — the factory verifies runtime receipts on the base branch after merge;
  deployment is the repo's own CI; only `health`/`rollback` are declared cmds.
- **Rebase-based conflict resolution** — merge-from-base only.
- **Referenced-but-missing metaswarm scripts.**
- **An MCP server** (standing decision).

---

## 12. Decisions, risks, and dogfooding

**Decisions** (status as of 2026-09-05)
1. Rename `sdlc-loop*` → `fix-loop*`: **deferred to Phase 7b**. *Accepted by Dave, 2026-09-04.*
2. Family state = namespaces + atoms over `NodeOutputs`/`Records` (chosen) vs typed `Delta.Family` (rejected).
3. Composition = driver-level with verified child output (chosen) vs in-machine nesting (rejected).
4. Runners = `runner:` on host kinds + class-`_RUNNER` fallback under `factory run` (chosen).
5. **Default routing profile (§5.4) ships with `pi` + GLM-5.3-flash via lunaroute as the `CURATOR` arm**, Claude
   elsewhere — and `setup --install-adapters` **asks the installing operator, per role class, which model and
   harness (runner) to use**, offering these as the defaults, so the profile is a chosen configuration rather
   than an inherited one. *Accepted by Dave, 2026-09-05.*
6. Sandbox mechanism per platform; `--unsafe-no-sandbox` exists, stamps the job, disables GitHub writes and
   merge; scripted exits may use it. *Accepted by Dave, 2026-09-05.*
7. Signed answer files (`ssh-keygen -Y`) and GitHub approval are both privileged paths — **both built, and
   both behind a job-level feature flag** `policy.answers: signed | agentic` (default `signed`): under
   `agentic` every privileged choice accepts the ordinary local CLI with `--by` as audit metadata and
   `merge-check` drops the external human-approval condition, so a fully agentic environment can run
   end to end without a person. `agentic` is recorded in the job record and `factory status`, and 12.8's
   "human at merge" remains the *default*, not an invariant. *Accepted by Dave, 2026-09-05.*
8. **A human is always required at merge**; "lights-out" = up to PR-ready. *Accepted by Dave, 2026-09-04.*
9. In-factory design = batched `author` + park; Socratic brainstorming stays a host skill. *Accepted by
   Dave, 2026-09-05.*
10. **`prove` is not a node of the `build` family** (`verify-tdd` + `mutate` subsume it; the review child's
    bug-family `prove` is unchanged). *Accepted by Dave, 2026-09-04.*
11. **Egress:** a credential-injecting driver-owned proxy with per-invocation sockets and an in-sandbox
    bridge (chosen) vs key-in-sandbox + domain allow-list (rejected: exfiltratable; no hostname allow-list
    in `sandbox-exec`/`bwrap`). Residual: a second runtime service on the critical path of smoke exits.
    *Accepted by Dave, 2026-09-05.*
12. **Curation default `auto`**; `human` parks only when there are proposals. *Accepted by Dave, 2026-09-05.*
13. **Branch protection is a prerequisite only for `merge: auto`**. *Accepted by Dave, 2026-09-05.*
14. **Beads DB isolation.** `close-beads` before merge assumes `bd` can use a per-worktree database; if
    Phase 1 verification shows it cannot, the epic close moves to `release-loop` after merge (and lands via
    the next job's PR). Fallback pre-approved so Phase 1 is not blocked on the spike. *Accepted by Dave,
    2026-09-05.*
15. **Post-merge learning is state-dir only.** The factory's `learn` node writes proposals to
    `<state-dir>/learning/<job>/`; they become committed rows only through the *next* job's
    `curate-commit` (their PR carries them). The interactive `learn --post-merge` is unchanged (it still
    renders `docs/metareview/learning/` and appends to the knowledge table in the checkout). A repo whose
    last factory job has finished therefore holds its final learnings only in the state dir until another
    job runs — recommended; the alternative (a learn branch + PR per job) was dropped in round 4 for having
    no lifecycle. *Accepted by Dave, 2026-09-05.*

**Risks**
- *Scope.* Eight epics; mitigation: each exit is scripted + smoke, and from Phase 2 the factory builds its
  successors.
- *Adapter and driver drift.* Adapters are data with recorded fixtures and a `health` pre-job check.
- *Sandbox portability.* `sandbox-exec` is deprecated; `bwrap` needs userns; containers are the supported
  fallback; the self-test decides `sandboxed`, never an assumption; scripted exits don't depend on it.
- *Judge cost.* Routing floors and the `usd` breaker bound it. Each `finalize` costs one fix-capable
  whole-branch review (skipped when there are no goldens), a recheck if it fixed, two review-only passes
  (`epic-check`, `pr-check`), and the two deterministic gates — once per shepherd cycle that commits; large
  diffs run in shard mode (one lens pass per shard + cross-shard). `pr-observe` blocks per change, not per
  poll, so a quiet PR costs nothing.
- *False confidence.* Self-reported fields are enumerated and cross-checked: `implement.status` vs
  `commits`/`implement_rejected`; `tests_added`, `report_path`, `commits`, `author.base` re-derived or
  validated; `author.questions` only parks; `curate.accepted` is rubric-, trust-, and policy-filtered;
  every child output is re-derived from the child's own audit. Every other gate input is engine-minted.
- *Trust boundary.* §5.12; the driver never runs git in, or checks agent content out with global config
  into, any tree; the platform jobs test it positively and negatively.
- *External verbs* (`bd`, `gtg`) verified at decomposition time; Beads DB isolation is decision 14.

**Dogfooding plan**
- Phases 0a/0b are built with today's flow (`review artifact`; `sdlc-loop-clean` per diff; gates with
  hand-run `record-lenses`).
- Phase 1's diffs are **reviewed and fixed** by Phase 0's unattended driver; its tasks are implemented by
  the interactive host.
- From Phase 2 each phase's tasks are **built** by `task-build-loop`; from Phase 3 each decomposition is
  authored by `artifact-loop`; from Phase 4 each PR is shepherded by the factory.
- The recall yardstick is recorded per phase in `docs/metareview/learning/`.

---

## 13. Coexistence, migration, compatibility, rollback

**Coexistence with metaswarm.** The factory owns the lifecycle **only for jobs it starts**. In a
`metaswarm-extension` repo with no factory job, metareview keeps today's gate-only contract. A factory job
in a metaswarm repo writes Beads through `BdRunner` and owns its own branch/PR; metaswarm's skills are not
invoked. The five documents are updated in Phase 0b.

**Schema policy.** `run.SchemaVersion` stays **1**; every change is additive: optional fields
(`InitData.{InvokedBy, Policy, Cmds, Consent, InitialFixEntry}`, `WarnData.{state, iter, attempt}`,
`TransitionData.{ToFixEntry, explicit_fail}`, `needs_input.{exec, runner, model, effort, attempt, from, …}`,
`process_call` on `node_output`, `Finding.Labels`/`Bug.Labels`, `Snapshot.{NodeAttempts, LastProcessError,
AnswerWindow, Policy, Records}`, `Golden.SourceID`, `child-spawn.golden_sources` all `omitempty`), new
driver-reserved `record` names (§5.3, incl. `answer-relay`, `child-park`, `import-park`), job-record
`task-state` rows, machine-reserved `mrv_exec_override`, new outcomes (`built approved released split
cancelled`, classes of §5.3), new `runs.jsonl` verdicts/statuses (`SPLIT`/`split`,
`CANCELLED`/`cancelled`), new `findings.jsonl` statuses `waived | accepted-risk | deferred` with
`dispositionBy/At/Reason`, `deferredTo` (`Blocks` stays `open | override-pending`), marker rows gain
`verdict` semantics (`PASS_ADVISORY` when dispositioned) and `DispositionedFindingIDs`, new YAML keys
(`family`, `runner`, `max_node_attempts`, `cmds_from_job`, `outputs`, multi-`loop`, `context`, `@from`,
new params). `epic-review-loop` gains an optional `CONTEXT` var, so its embedded hash changes — a
workflow-consent change like any other (in-flight jobs park on `consent-park`). Pre-revision
`converge` events carry `class: overflow` where new runs say `budget`. The new binary folds every existing
audit (golden replays of pre-refactor fixtures are the Phase 0b exit; no folded field derives from event
times except `AnswerWindow.at`, absent in logs without answers). An **old** binary fails closed on new
logs/YAML; rollback rule: finish or cancel in-flight jobs before downgrading; `.metareview/jobs/`,
`adapters/`, `models.yaml`, `sandbox/`, `refsrc/`, `refs/metareview/*` are inert to an old binary. **CI must
pin `metareview ≥ <version>`** in repos carrying plan-rubric review logs.

**Review logs.** The artifact-loop terminal handler writes the existing header contract plus `Rubric:`,
chaining `--previous-run` per target across the loop's `artifact-review-loop` children (a job-record
`review-run {target, run_id}` row, like `gate-run`), so an earlier iteration's `NEEDS_REVISION` log on
the same document is superseded, not left in branch scope for the hook; lens sets are fixed per
registered rubric by the binary and honoured only when the artifact's content class matches, so a
header cannot reduce a spec log's requirement. Legacy hand-edited logs keep parsing and
blocking when `NOT_REVIEWED`; `review artifact` without a runner stays usable interactively.

**Markers.** `record-lenses` survives; both writers share the head rule. **Reader change (Phase 0b):**
`LatestReviewEvidence`, `validateFromRunDiff`, and `record-lenses` adopt the marker-currency rule of §5.3
(the pre-push hook reads logs, not markers, and is unchanged) — a marker whose `headSha` is an ancestor of HEAD with only diff-excluded
paths changed since is current. Existing markers are unaffected (an exact match is a current match);
the only behavioural change outside a job is that a commit touching only `docs/metareview/**` or
`.metareview/**` after `record-lenses` no longer requires re-recording — which is what the CLAUDE.md
"re-record after any new commit" rule becomes.

**Receipts.** `schemaVersion` 1; `sourceRevision` optional; outside a factory job unbound receipts remain
accepted indefinitely; inside, only `verify-tdd`/`validate`-minted receipts count; ancestor test necessary,
exact-match sufficient (§5.8).

**State roots.** Existing users who ran gates inside worktrees: `state migrate` (terminal runs only); the
two-root rule changes callers, not `repo.Root` semantics for checkout content.

**Workflow names.** Unchanged until Phase 7b.

**Knowledge.** One table per repo mode (§5.11); dual-root reads; existing `.beads/knowledge/` rows preserved;
append-only with `supersedes`.

**Durability table (new files):**

| Path | Committed? | Writer |
|---|---|---|
| `<state-dir>/jobs/<id>/job.jsonl` | no | `factory` |
| `<state-dir>/{models.yaml, adapters/*, sandbox/*, gitconfig}` | no (materialized) | `setup --install-adapters` |
| `<state-dir>/refsrc/<job>/`, `<state-dir>/learning/<job>/` (+ its `consumed` marker), `<state-dir>/jobs/<job>/{cache/, evidence.md, spool/<esc>.json}` (`cache/` and `spool/` removed by `factory gc` with the job's `refsrc/`; `evidence.md` kept with the job record) | no | `factory`, `learn`, the driver, `cmds.install`, `gate-review` |
| `docs/metareview/fsm/<id>/` (children by reference; `--deep` nests, per-run cap) | yes (export) | `fsm export` |
| `docs/metareview/learning/*` | yes | `learn` (interactive) |
| the knowledge table (`knowledge.Path`, driver worktree; rides the PR) | yes | `curate-commit`, `learn` |
| `.worktrees/*` | excluded (`.git/info/exclude`) | `internal/worktree` |
| `.metareview-report/` (clone, `.git/info/exclude`) | no | `implement` |
| `refs/metareview/{discarded,base-tip}/*` (main repo; never pushed — `push` argv is fixed); `refs/metareview/{mirror-head,base-tip}` (clone) | no (refs) | driver; the mirror |

metaswarm's `.beads/plans/active-plan.md`, `.beads/context/*.md` are neither written nor read.

---

## 14. Future directions recorded, not built

**14.1 Visual design, prototyping, and user-feedback loops in the design/specification phase.** A stage
inside brainstorm → design → spec that produces *usable skeletons* (clickable prototypes, throwaway UI,
stub services) so real users can give design and usability feedback **before** the heavyweight build,
iterating quickly without full functionality and anchoring the spec in designs and observed use. It is
human-in-the-loop by nature and needs **telemetry and instrumentation** about usage and product
capability as first-class evidence. Not part of this plan; not in metaswarm or metareview today.

How it maps, and what to keep open:
- *Carrier:* a third `artifact`-family loop, `prototype-loop`, with `author.template: prototype` and a
  usability/visual rubric — `rubric:` names an embedded rubric and `author.template` is a registry, so
  neither may become a closed enum.
- *The human turn:* an `await` code `ESC_USER_FEEDBACK{prototype_url, instrument_ids}` that is *expected*
  to park — the `ESC_*` registry is per family and data-driven (§5.3).
- *Evidence:* a receipt kind `telemetry` alongside `validation`/`runtime`/`ci-check`; `Receipt.kind` is a
  string and `HasSuccessfulValidation` selects by kind, so a telemetry kind that never satisfies a
  validation query is additive; `design_anchored`/`feedback_anchored` atoms over receipts.
- *Artefacts:* design files referenced from the spec by content hash; context packs must be able to hash
  binary/large artefacts rather than inline them.
- *Loop shape:* brainstorm ↔ prototype ↔ feedback ↔ spec is multi-loop — §5.3's grammar expresses it.
- *Instrumentation of the prototype itself* is product work outside metareview; the factory consumes receipts.

What this plan must not do: close the `author.template` registry; make the `ESC_*` list a compile-time
enum; assume every receipt kind is a test run; assume the design stage is one `author` → one review.
§8.6 owns these constraints.

## 15. Review record

Revision 1 was reviewed by nine parallel lens subagents against `rubrics/artifact-review-rubric.md` (run
`mrv-20260905-055717529859000-artifact-2026-09-04-lights-out-sdlc-factory-e7ed2a69`): 59 blocking findings.
Round 2 (revision 2): Scope and Intent PASS; 61 blocking. Round 3 (revision 3): Scope and Intent PASS;
33 blocking, strongly convergent, incl. a live-verified P0 (driver-side git in the agent clone executes
agent config). Round 4 (revision 4): Scope and Intent PASS; 31 blocking in five convergent clusters —
cross-loop interpolation (4 lenses), `loop_converged_edge` vs the outlines (4), `child_failed` (3), the
final-chain ordering / `entry_ship` (3), `$JOB_BASE` drift after merge-from-base (2) — plus single-lens
catches verified live: agent-committed `.gitattributes` selecting the driver's global git drivers at
import, `-c diff.external=` making any `diff=` attribute fatal, `origin/<base>` in a `--shared` clone being
the main repo's local branch, and `*_BASE_URL` unable to address a unix socket. Revision 5 (this document)
resolves each cluster with one rule: latest-at-or-before interpolation with defaults (§5.3);
`cycle_without_converged` + precedence with forward edges before `converged` (§5.3); the outcome→atom map
incl. `child_failed` (§5.3); the shared `finalize-loop` entered after every commit (§6); `JOB_BASE` as a
re-resolved merge-base SHA (§5.3); driver git with `GIT_CONFIG_GLOBAL=/dev/null` and the protected-path
check at import (§5.9); the egress bridge (§5.4); `refs/metareview/base-tip` in the ref source (§5.9); plus one
knowledge table with dual-root reads (§5.11), the learn branch dropped, credential-bearing cmds in their
own sandbox instance with a raw-tail secrets scan, per-invocation proxy sockets, sandboxed upload-pack,
`@from` retries, driver-reserved records, `AnswerWindow`, the `simple-task`/`decomposition` path
convention, and decisions 12.14 and the acceptance status of 12.9.

Round 5 (revision 5) was cut short by a session limit: Scope, Intent, and Testing-quality PASS;
Architecture found four blockers — the factory never ran the deterministic gate *commands* (so the
git-native pre-push hook would block its own push), `override → mark-epic` could not mint, latest-output
interpolation picked the stale channel at merge points, and whole-branch review had no sharding in the
FSM; the other five lenses did not report. Revision 6 added the `gate-review` node and the three-bases
rule (§5.3, §6), override-as-disposition with re-review (§5.3, §6), `${@from.<field>}` and the split
CI/triage finalize routes (§5.3, §6), `review-lenses` shard mode (§5.3), `child_failed` widened to every
non-escalated non-pass leaf, durable `task-state` job rows, `answer-relay`, `INTEGRATE`,
`markers_current`, `SHEPHERD_OBSERVE_MAX` → `pending`, `open-pr` on CLOSED, `fixed_golden_idx`,
`base-tip`, `JOB_BASE_REF`, `no-tests` restricted by `taskset_valid`, the secrets-scan scope, and decision
12.15.

Round 6 (revision 6; the six unfinished lenses): 20 blocking, in one dominant cluster and a tail. **Five
of six lenses** independently found that `gate`'s log commit moved HEAD past the markers `mark-epic`/
`mark-pr` had just minted, while every marker reader was exact-head — so `markers_current` was never
true and `merge-check` could never be satisfied. Revision 7 (this document) answers it with **one
marker-currency rule** (§5.3: ancestor + only diff-excluded paths changed since) implemented once and
used by every reader, `record-marker`, the hook, `merge-check`, and a new `marker-check` fork kind (atoms
are pure over `Snapshot`). The tail, each fixed where named: `gate-review` chains `--previous-run` per
scope (§6, §7); the goldens grammar (`$VAR` sources, concatenation, `Golden.SourceID`, `{file, line,
desc}` shapes, node-less `@from` → default) and the **identity join** for dispositioned bugs
(re-supplied as trailing goldens; §5.3) — the round-5 override → re-review fix was not mechanically
closable without it; `await` echoes its `context` so `task-build-loop`'s goldens have a producer (§6, §7);
non-bug workflows declare an `outputs:` map verified against their leaf (§5.3) — `fixed_golden_idx` and
`advisory` were structurally empty at `finalize-loop`/`epic-build-loop`; `fix`/`fix-integration` receive
`validate.failures`/`gate.findings` as goldens (§6, §5.8); `factory.ship` routes `child_pass` (its
bug-free `reviewed` leaf never matched `child_reviewed`); an unrouted `child_failed` is a `child-park`
driver park rather than a dead job, and the import's `ESC_PROTECTED_PATHS` an `import-park` (§5.3);
`loop_source_without_converged` replaces the per-cycle rule and `gate`/`push-ci` gain `converged` edges
(§5.3, §6); `ALLOWED_PATHS` is engine-implicit so suite-mode validates see the job's allowances (§5.3,
§5.8); the shepherd's `PrState` atoms incl. `ready` are defined in evaluation order (§5.10);
`resolve-threads` moves to the fix route with `pr-resolve` mapping through `golden_sources` (§6, §5.10);
`ESC_FINALIZE_ESCALATED`, `ESC_PUSH_REJECTED`, `ESC_CHILD_FAILED`, `ESC_ROLLBACK.extend` (§5.6).
Verified live by Feasibility and Security: `git worktree add --detach` cannot materialise a throwaway
tree from a read-only ref source (→ `clone --shared` + `checkout --detach`, §5.9); the mirror's bare
refspec never created `refs/metareview/base-tip` in the clone (→ destination refspecs); and — the one
Security blocker — the driver's gate wrote through an agent-committed symlink under the unprotected,
diff-excluded `docs/metareview/` (→ `docs/metareview/**`, `.beads/**`, the knowledge table and every
symlink protected at import; `Lstat`+`O_NOFOLLOW` for every driver-side worktree access; §5.9, §5.12.4),
with its warnings absorbed: the driver-owned `gitconfig` for `push`, the base-tip blob exemption at
import, a privileged reopen after a maintainer's close, the bridge token on shared macOS loopback,
tree-resident shard results ignored under a job, `{pid, start_time}` and the `allowed_signers` copy in
the job record.

Round 7 (revision 7; seven lenses): Security PASS (the symlink write-through verified closed live); the
others found a short, mostly live-verified tail. Feasibility: the mirror's unforced refspecs reject the
backwards restore (→ `+` refspecs); `--name-status` overrides `--raw` and hides the `120000` mode the
symlink check needs (→ `--raw --no-renames`, mode column); `push` used the shepherd's folded `$JOB_BASE`
as the hook's base — the one use §5.3 forbids (Mechanical found the same) → `push` computes
`merge-base(origin/<base>, HEAD)` live; throwaway trees carry no dependencies → the job dependency cache
and `cmds.install` inside the throwaway at its own head (§5.9). Data-migration (live): the driver's
`.beads` pre-resolution re-conflicts when the agent re-runs the merge → the agent takes ours for the
driver-resolved set and the import refuses those paths unless their blob equals the driver head's.
Architecture: a trusted thread triaged `reply_only` could never be resolved, making `ready` unreachable
→ every triage route ends in `reply → resolve-threads`, which also resolves replied trusted threads;
`outputs:` was latest-only → `${state.field@all}` (and `@iter` for the opposite need); review-log
supersession does not cross child runs → `gate-run`/`review-run` job rows. Testing-quality: the scripted
Phase 4+ exits needed GitHub writes the mandated `--unsafe-no-sandbox` disables → the faked `Sandbox`
seam stated once in §8.2, plus per-reader stale fixtures, the enumerated Phase 4 thread fixture, and
the `matrix.absent`/`consumed`/`ERR_JOB_EXISTS`/cancel-during-observe exits. Mechanical: `ALLOWED_PATHS`
was a frozen var → a projection over job-record `allowance` rows; `verify-tdd` moved to shared;
`merge-check` accepts `PASS_ADVISORY` and a UI merge; `ready` requires `OPEN`; the machine attaches
`await.context` at answer time; dispositions reach `confirmed_fixable`/`recheck`; `issue-skipped`
receives task ids. Completeness (code-verified): the gate commands demand evidence nothing in the
factory produced — `review pr-ready` blocks on `pr:missing-validation-evidence` and Beads-mode `review
epic-ready` on `epic:missing-child-evidence` → `gate-review` renders the evidence file from
`validate.receipts` and the job record's `task-state built` rows; plus `retry_narrowed` closing
projected findings, `factory.build`'s failure parking instead of failing, `resplit`/`revise` routing
every `author` outcome, `DOC_PATH` = `plan_path` when supplied, `rollback`'s exit routed, a
maintainer-closed PR ending neutral, `treat_as_real` as a fixable disposition, `mutation_survivors`
defined as the complement. Revision 8 carried all of it.

Round 8 (revision 8; six lenses): Mechanical PASS; the rest found a live-verified tail. Feasibility:
the driver pre-resolved only `.beads/**` while the import demanded the whole driver-resolved set equal
the driver head → the driver merges the whole set itself (union for the knowledge table, both sides for
`docs/metareview/**`) and the agent restores those blobs by explicit checkout (Data-migration showed
`--ours` silently skips non-conflicted paths); the lockfile-hash skip of `cmds.install` left
`node_modules` absent → skip only for out-of-tree conventions; a fresh gate chain after an `ESCALATED`
log never released the hook → the chain now ends at a PASS row and the CLI cap sits above the finalize
window, so no `ESCALATED` log is minted (Architecture found the same from the attempt counter);
convention-default installs get fixed egress lists. Architecture: `fixable` dispositions fell under the
covering join → two disposition classes; evidence lines only for `built` failed every skipped/overridden/
split task in Beads mode → one line per terminal `task-state` row with the literal `pass` token and the
Beads id. Completeness: the `override` route never visited `validate`, so `gate`'s evidence had no
producer → `override → validate (loop)`; a maintainer's commit on the job branch was never fetched →
the `sync-branch` state; `rollback`'s new loop edge lacked `converged`. Testing-quality: the faked
`Sandbox` seam had no injection path → `-tags factorytest`; Phase 2 filed issues under the flag that
disables writes → the faked seam for any write-node exit; per-kind `dry_run` negatives; the ordered
T1–T4 fixture; the `.beads` theirs negative; the install call-count exit. Data-migration: the learning
`consumed` marker becomes a run-store projection with `unmerged` un-consumption; `gate-run` rows are
written before the commit; the durability table gains `evidence.md`/`spool/`/`cache/`. Also from the
advisories: `resolve-threads` pairs indices with the goldens list from the *same* child output (the
child output gains `goldens[]`; `@iter` dropped there); `task-build-loop` declares `outputs:` and a
`GOLDENS` var so `retry_narrowed` re-supplies projected findings; `ask-integration`
(`ESC_INTEGRATION_ESCALATED`) separates integration escalations from task ones; `ask-split` for a
split author's questions; `factory.build` relies on the `child-park` default; `file-issue` idempotent
per thread and `pr-reply` per filed thread; `merge-check` blocks on the poll; the dispositions source
sits second so truncation never cuts it and `ERR_GOLDENS_OVERFLOW` bounds `$GOLDENS`; `author` shared;
`size_simple` only for building intakes. Revision 9 carried all of it.

Round 9 (revision 9): Completeness PASS (every round-8 blocker verified in text, the gate-override path
in code); Security found one blocker — the writable job cache contradicted §5.12.1's exact writable
set and bridged the "credentials, no network" and "network, no credentials" invocations → read-only in
credential-bearing invocations (revision 10, this document), plus `sync-branch`'s fetch named as the
third network operation and `ready` refusing `CHANGES_REQUESTED`. Five lenses were cut short by a
session limit. The review was **closed at this point by the author**: nine rounds had converged from 59
blockers to one, and the remaining Completeness advisories (missing exits for the newest states, an
`ask-integration` note carrier, the diverged-remote reconciliation, `ask-matrix` context, a scheduler
convergence code, empty escalation contexts) are recorded in the review log as accepted advisories to
resolve during Phase 0b/2/4 implementation rather than in further spec rounds.
