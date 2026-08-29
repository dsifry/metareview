# metareview 0.9.0 — FSM Core Design Plan

> **Status:** DRAFT for review. This is the agreed scope and interface sketch for the 0.9.0
> release of `bin/metareview` (the metaswarm review binary). It is a design plan, not an
> implementation. Build nothing until the schema and interfaces below are reviewed and locked.
>
> **Origin:** This design was driven by running the SDLC-loop experiment
> (`harnesseval/sdlc_loop.py`) and hitting three concrete failures the current review-only
> binary could not prevent: (1) context thrash from re-spawning `claude -p` per phase
> (5.2M tokens, mostly cache re-warm), (2) a silent 0-fix score because an agent edited 27 files
> but committed none, and (3) no way to swap the adjudicator model to validate whether gpt-5.2
> mis-classifies real bugs as hallucinations.

## 1. Guiding principle

**Deterministic workflow structure + auditable, configurable, but still nondeterministic LLM
calls.** The FSM guarantees: "you cannot transition FIX→VERIFY without a commit and an
adjudication" and "every transition emits an error code or a state change." The FSM does
**not** guarantee: "the adjudication was correct." Conflating those would over-promise.

The win: the *structure* becomes reproducible and the *LLM calls* become swappable+auditable —
which is exactly what lets us validate the gpt-5.2 concern (swap `claude-opus-5` into the
adjudicate node and diff the verdicts against the same run with gpt-5.2).

## 2. Architectural shift

The loop moves **out of orchestrator Python** and **into the warm agent session.** The agent
(Claude/Codex/GLM) is the executor; the `metareview` binary holds the FSM state and enforces
gates. One warm session across discover→fix; only cross-model judge calls leave the session
out-of-band. This is what kills the 5.2M-token context-thrash and the single-subprocess-failure
cascade we just watched take down a run.

The binary already has the germs: run-ids (`mrv-...`), `--previous-run`, `--max-attempts`, and
`Gate effect: advisory`. 0.9.0 promotes those from review-only scaffolding into a real state
machine.

## 3. Execution modes (the warm-context mechanism)

This is not a separate feature — it is a property of the node spec. The `exec` field determines
how a node runs, and it is the mechanism that delivers warm-context co-hosting:

| `exec` | When | Context | Use for |
|---|---|---|---|
| `inline` | the host agent runs the node in its own session | **warm** (full lived context) | `agent-edit` (fix) — the host carries discovery's understanding straight into the edit |
| `subagent` | the host spawns parallel sub-agents | warm-ish (gets task input, re-derives) | `review-lenses` (8 lenses in parallel) |
| `fork` | the binary runs a separate process | **cold** (fresh spawn) or out-of-band HTTP | cross-model `judge` (a non-session model cannot live in a Claude session) |

**Default by `kind`:** `review-lenses`→`subagent`, `agent-edit`→`inline`, `judge`→`fork`.
Overridable per node. The critical invariant: **same model = in-session (subagent for
parallelism, inline for warmth); cross-model = fork.** Only the judge call forks; everything
else stays warm. Making `fix` a subagent would lose exactly the warmth we're keeping, so
`agent-edit` defaults to `inline`.

## 4. CLI surface (no MCP — CLI tools work across all three hosts)

```
mrv fsm init   --workflow <name> [--var KEY=val...]        → run-id + initial state + valid transitions
mrv fsm state  [--run <id>]                                → current state, pending gates, last error
mrv fsm advance [--run <id>]                                → attempt next transition; runs deterministic gate checks;
                                                              if a gate needs an LLM call, calls it per config OR
                                                              returns NEEDS_INPUT for the agent to satisfy
mrv fsm gate   <name> --input <file>                        → run one named gate; pass/fail + error code
mrv fsm judge  --model <m> --effort <e> --kind <k> ...      → the configurable LLM-as-judge primitive (cross-family)
mrv fsm record <event> --data <json>                        → agent logs what it did (fixed X, committed Y)
mrv fsm converge --check <predicate>                        → deterministic convergence test over state
```

All commands are CLI so they work inside an interactive `claude -p`, `codex`, or `glm` session
as tool calls. No MCP server for 0.9.0.

## 5. Workflow YAML schema (declarative — the actual product)

```yaml
workflow: sdlc-loop
version: 1

# Variables: defaulted, overridable at `fsm init --var KEY=val`. This is the swap surface.
# Re-running with JUDGE=claude-opus-5 reproduces the entire workflow with a different adjudicator.
#
# IMPORTANT — two judge defaults, do not conflate:
#   - CALIBRATION JUDGE = gpt-5.2. Used ONLY to reproduce Martian's published eval numbers.
#     Pinned; changing it breaks eval calibration. Set via `--calibration` mode, NOT the product default.
#   - PRODUCT DEFAULT JUDGE = <TBD by the judge-eval>. The 0.9.0 binary's default `JUDGE` var is
#     whatever our separate judge-eval determines is Pareto-optimal on recall/precision/AUC/R²
#     balanced against token cost and wall time — NOT gpt-5.2 (that's the eval's calibration default,
#     not a product choice), and NOT the absolute best/highest-effort model (don't burn tokens to
#     eke out a tiny AUC/R² gain). The judge-eval sweeps our available models × efforts and picks
#     the knee of the Pareto frontier. Until that eval reports, the binary ships NO hardcoded
#     judge default — `fsm init` REQUIRES `--var JUDGE=<model>` (fails with ERR_JUDGE_UNSET).
vars:
  REVIEWER:    {default: claude-opus-5}
  JUDGE:       {required: true}          # no product default until the judge-eval picks one (§17)
  REV_EFFORT:  {default: low}
  JUDGE_EFFORT: {default: medium}      # also TBD by the judge-eval; medium is a placeholder

states: [discover, adjudicate, fix, verify, done, failed]

transitions:
  discover→adjudicate:  {gate: findings_nonempty}     # error: ERR_NO_FINDINGS
  adjudicate→fix:       {gate: confirmed_nonempty}   # error: ERR_NO_CONFIRMED
  fix→verify:           {gate: commit_exists}        # error: ERR_NO_COMMIT
  verify→discover:      {gate: bugs_remain, converge: <predicate>}   # loop
  verify→done:          {gate: all_fixed}            # converge
  *→failed:             {on: gate_error}             # any gate failure

# Nodes: each references a kind (from the plugin registry) and an exec mode.
nodes:
  discover:
    kind: review-lenses
    exec: subagent          # 8 lenses in parallel (same model)
    lenses: 8
    model: $REVIEWER
    effort: $REV_EFFORT
  adjudicate:
    kind: match-then-adjudicate
    exec: fork              # cross-family judge — must leave the session
    model: $JUDGE
    effort: $JUDGE_EFFORT
  fix:
    kind: agent-edit       # NOT an LLM call — the host agent edits; the FSM gates it
    exec: inline           # warm: host carries discovery's context into the edit
    executor: $SESSION     # the host agent itself, not a spawned model
  verify:
    kind: still-present
    exec: fork             # cross-family judge
    model: $JUDGE
    effort: $JUDGE_EFFORT

# Convergence: first-class user-facing composable atoms. Decides loop-vs-done at
# the verify→discover / verify→done boundary. Deterministic — never punted to the agent.
convergence:
  any:                                       # compose primitive
    - all_fixed                              # atom (built-in)
    - no_fixation_progress                   # atom (built-in)
    - { budget: { tokens: 4_000_000 } }       # atom with params
    - { max_iterations: 5 }                  # atom with params

# Repo ownership: advisory default (agent edits freely, advance diffs and flags
# unsanctioned changes as WARN: UNSANCTIONED_EDIT). Enforcing opt-in: all writes go
# through `mrv fsm fix-commit`; direct git edits are a gate violation.
repo_mode: advisory   # | enforcing

# Overflow handler (the ONE sanctioned `cmd:` use in a default workflow).
# Fires ONLY when a built-in safety stop (max_iterations / budget) triggers — never as
# part of normal logic. A fallback action, not a logic node: cannot change state, only
# react. Same guardrails as any cmd: (opt-in, declare+verify, timeout, audit). Optional.
on_overflow:
  cmd: "./notify-overflow.sh"   # e.g. dump state, post to CI, file a ticket
  timeout: 30
```

## 6. Multi-kind classifier plugin system (in 0.9.0)

A node's `kind` references a registered classifier. The registry is the swappable surface.
Built-in kinds for 0.9.0:

| `kind` | Input | Output | exec default | Notes |
|---|---|---|---|---|
| `review-lenses` | diff + lens defs | `list[Finding]` | subagent | dispatches N adversarial lenses in parallel (same model) |
| `match-then-adjudicate` | `list[Finding]` + goldens | `{matched, real_but_ungold, hallucination}` | fork | the cross-family judge — matches vs goldens, adjudicates unmatched |
| `still-present` | bug desc + current diff | `{still_present: bool, confidence}` | fork | fixation check per bug |
| `agent-edit` | confirmed bugs | commit (enforced by `commit_exists` gate) | inline | the host agent edits; NOT an LLM call |

**Plugin interface (so kinds are swappable):** a kind is registered with:
- `name`: string
- `run(input, config) -> output` — the implementation (Go for built-ins; registered at build)
- `schema`: input/output JSON schema (for validation + audit)
- `default_exec`: `inline | subagent | fork`
- `llm`: bool — whether this kind makes an LLM call (governs whether model/effort apply)

A user-defined kind registers via a config file pointing at a binary/script (the same
shell-command escape-hatch pattern as convergence). This generalizes later; for 0.9.0 the
four built-ins are the product.

## 7. LLM-as-judge primitive

The single highest-value piece — this is what unblocks the gpt-5.2 validation:

```
mrv fsm judge --model <model> --effort <effort> --kind <kind> \
              --input <file> --context <file> [--run <id>]
```

- **Cross-family HTTP call** (the configured JUDGE model from a session of a different family — e.g. an OpenAI judge from a Claude session, an Anthropic judge from a Codex session).
- `--kind` selects the judge prompt family (adjudicate / still-present / match) — same prompt
  the eval uses today, now a first-class primitive.
- Every call is **audited**: model, effort, kind, input hash, verdict, confidence, timestamp
  written to the run's audit log.
- Swappable via the `JUDGE` var: `JUDGE=claude-opus-5` re-runs the same workflow with a different
  adjudicator and the verdicts are diffable run-to-run. The product default is the Pareto-optimal
  pair from the judge-eval (§17), not gpt-5.2.

This turns the ad-hoc `bin/rejudge_hallucinations.py` validation into a re-runnable primitive:
re-init the workflow with a different `JUDGE`, advance, diff the two runs' audit logs.

## 8. Deterministic gates (with error codes)

Gates are pure functions over FSM state — no LLM, no network. They pass/fail and emit an
error code on fail. Built-ins for 0.9.0:

| gate | passes when | error code |
|---|---|---|
| `findings_nonempty` | discover produced ≥1 finding | `ERR_NO_FINDINGS` |
| `confirmed_nonempty` | adjudicate produced ≥1 confirmed bug | `ERR_NO_CONFIRMED` |
| `commit_exists` | `git rev-list --count base..HEAD` > 0 AND working tree clean | `ERR_NO_COMMIT` |
| `all_fixed` | verify reports 0 still-present bugs | (not an error — converge signal) |
| `bugs_remain` | verify reports ≥1 still-present bug | (transition signal, not error) |

`commit_exists` is the gate that would have caught the silent 0-fix bug deterministically:
the agent edited 27 files but committed none → `advance` returns `ERR_NO_COMMIT` with the
diff of uncommitted changes, instead of scoring 0 fixes silently.

## 9. Convergence predicate library (composable user-facing atoms)

First-class part of the workflow authoring surface. The FSM calls the predicate at each
verify→discover / verify→done transition to decide loop-vs-done. **Deterministic — never
punted to the agent** (punting would reintroduce nondeterminism at the one boundary we
promised determinism).

**Built-in atoms:**
- `all_fixed` — `n_unfixed == 0`
- `no_fixation_progress` — `n_unfixed >= prev_unfixed` (prevents infinite loop when the agent
  can't fix the last few bugs)
- `max_iterations: N` — `iter_count >= N`
- `budget: {tokens: N}` — total token spend ≥ N (prevents the 8.6M-token Codex runaway)

**Compose primitives:** `any` (stop if any fires), `all` (stop if all fire), `not`.

**Escape hatch (no DSL):** a custom atom is a shell command the FSM runs with the state
snapshot on stdin, returning `{stop: bool, reason: str}` on stdout:
```yaml
convergence:
  any:
    - all_fixed
    - { cmd: "./stop-when-recall-plateaus.sh", window: 2 }
```
The built-in atoms + compose cover 95%; the shell-command atom covers the long tail without us
ever building a predicate expression language.

## 10. Run persistence + audit log

Every run has a run-id (extend the existing `mrv-...` model). Persisted:
- State transitions (from → to, gate result, timestamp)
- Every gate decision (gate name, passed/failed, error code)
- Every LLM call (kind, model, effort, input hash, verdict, confidence, tokens, timestamp)
- Agent-recorded events (`mrv fsm record fix --data '{"bugs": 8}'`)
- Working-tree diff snapshots at each `advance` (for `WARN: UNSANCTIONED_EDIT` detection)
- **Node outputs** (the result of each node — e.g. the confirmed-bugs list, the adjudication
  verdicts, the fix commit hash) so a node can be re-run from its persisted inputs without
  re-running upstream nodes (see §10.1)

Audit log is plain JSONL under `.metareview/runs/<run-id>/`. This is what makes runs
diffable: two runs with different `JUDGE` values produce runs whose adjudication verdicts can be
diffed cell-by-cell. (No web/CLI dashboard for 0.9.0 — the JSONL is enough; visualization is 0.9.1+.)

### 10.1 Resume / re-run from checkpoint

Persistence is half a feature without resume. The whole point of persisting state is to continue
from it — and the FSM's determinism promise depends on it: if a gate fails mid-run
(`ERR_NO_COMMIT`), the correct operator action is "fix the commit, resume from verify" — not
"re-run the whole workflow." Without resume, every gate failure is a full restart, which makes the
deterministic gates punitive rather than helpful.

We proved the need empirically: a logic bug in one node (the fix-loop convergence predicate)
forced a full re-run of four unaffected phases (~4M tokens, ~20 min wasted) because the runner had
no checkpoint to resume from. Resume is what turns that from "re-run everything" into "re-run from
the node that changed, with upstream outputs frozen."

**CLI:**
```
mrv fsm advance --run <id> [--from <state>]   # resume: load persisted state, continue from <state>
                                               # (default: the run's current state). Frozen upstream
                                               # inputs (union_bugs, confirmed lists, commit hashes)
                                               # come from the audit log's persisted node outputs.
```

**Idempotency contract** — a node is re-runnable from its persisted inputs without re-running
upstream nodes. Per-kind:
- `review-lenses` (discover): idempotent given the repo + base ref; resume re-runs only this node
- `match-then-adjudicate` (adjudicate): idempotent given the persisted findings list; resume does
  NOT re-run discovery
- `agent-edit` (fix): idempotent given the confirmed-bugs list + a fresh repo copy (the agent
  edits a new work tree); resume re-runs the fix loop with the union frozen from the prior run
- `still-present` (verify): idempotent given the bug list + work dir; cheap judge calls only

**The rule for changing logic:** if you fix a node's implementation (e.g. the convergence
predicate), resume `--from` that node's entry state; everything upstream is replayed from the
persisted audit log, not re-executed. If you change the `JUDGE` var, resume `--from adjudicate`
(everything before the judge is model-independent and frozen; only the judge-dependent nodes
re-run). This is the precise mechanism for the §17 adjudicator-validation workflow (re-run with
`JUDGE=claude-opus-5`, resume from adjudicate, diff verdicts).

**Open question (§15.5):** formalize the idempotency contract per `kind` — which nodes are safely
re-runnable, which inputs must be frozen, which side effects (commits, files) must be reset.

## 11. Repo ownership mode

- **advisory (default):** the agent edits freely; `advance` diffs the working tree against the
  last recorded state and flags unsanctioned changes as `WARN: UNSANCTIONED_EDIT`. The gate
  still passes on a real commit. This is what makes the FSM usable inside an interactive
  session without neutering the agent as a collaborator.
- **enforcing (opt-in):** all writes go through `mrv fsm fix-commit`; direct git edits are a
  gate violation. Stronger determinism, but feels like a harness, not a collaborator. Hardening
  deferred to 0.9.1+; the flag exists in 0.9.0 but advisory is the supported path.

## 12. Warm-context usage pattern (documented + one worked example)

Not a feature build — falls out of the exec modes. The deliverable is:
1. A docs page: "Driving a workflow inside an interactive session" — the recommended loop an
   agent follows (`fsm advance` → read state → do the node's work → `fsm record` → `fsm advance`).
2. One worked example: the SDLC-loop workflow above, shown end-to-end in a `claude -p` session.
3. Optional: `mrv fsm run [--run <id>]` — a convenience wrapper that prints the loop skeleton
   (the agent still executes; the wrapper just emits the next-step prompt).

This is what stops users defaulting to the cold-context subprocess pattern (the obvious thing)
and paying the 5.2M-token tax we measured. The doc is cheap; the cost of *not* documenting it
is that every user re-discovers the thrash.

## 13. 0.9.0 scope — final

**In:**
- Declarative workflow YAML + `fsm init/state/advance` with error-coded deterministic gates
- Multi-kind classifier plugin system: node `kind` (`review-lenses`, `match-then-adjudicate`,
  `still-present`, `agent-edit`) as a swappable registry
- Node `exec` modes (`inline | subagent | fork`, inferred from `kind`, overridable) — the
  warm-context co-hosting mechanism
- Configurable LLM-as-judge primitive (`fsm judge --model --effort --kind`) — cross-family
- Deterministic gates with error codes: `findings_nonempty`, `confirmed_nonempty`,
  `commit_exists`, `all_fixed`, `bugs_remain`
- Convergence predicate library as composable user-facing atoms: `all_fixed`,
  `no_fixation_progress`, `budget`, `max_iterations`, compose with `any`/`all`/`not`,
  + shell-command escape hatch
- Warm-context usage docs + one worked SDLC-loop example
- Run persistence + audit log (JSONL)
- Advisory mode default, enforcing opt-in (flag exists; hardening deferred)

**Extension safety (escape-hatch guardrails)** — in 0.9.0 (see §16):
- Opt-in at `fsm init` (`--allow-custom-cmds`), never silent auto-exec from an untrusted file
- Declare + verify required `cmd:` paths at init (`ERR_CMD_NOT_FOUND`)
- Timeout + schema validation on every `cmd` call (`ERR_CMD_OUTPUT_INVALID`)
- Full audit of every `cmd` call (input hash, output, exit code, duration)
- **Default workflows ship built-ins only** — no `cmd:` nodes in normal logic; the one
  sanctioned `cmd:` use in a default is an `on_overflow` safety-stop handler (fires only when
  `max_iterations`/`budget` trigger), for notify/dump/ticket fallback (see §16)

**Deferred (0.9.1+):**
- MCP server
- Web/CLI dashboard over run history
- Enforcing-mode hardening
- Convergence predicate DSL / custom user-defined predicates beyond the shell-command escape
  hatch (the built-ins + compose cover 95%; the guarded `cmd:` atom covers the long tail)
- Additional classifier kinds beyond the four built-ins

## 14. What this directly fixes (the five failures that drove it)

1. **Context thrash (5.2M tokens, found 14 / fixed 5)** → warm-context co-hosting: the agent
   stays warm across discover→fix; only the cross-model judge call forks. Plausibly recovers
   the fixation gap because fix carries discovery's lived context.
2. **Silent 0-fix (agent edited 27 files, committed none)** → `commit_exists` gate returns
   `ERR_NO_COMMIT` with the uncommitted diff, instead of scoring 0 silently. Deterministic
   gates make failures *visible*, not *silent*.
3. **adjudicator validation (the gpt-5.2 concern)** → `JUDGE` is a config var; re-run the same
   workflow with a different `JUDGE` (e.g. `JUDGE=claude-opus-5`) and diff the two runs' audit
   logs. **Resume (§10.1) makes this cheap**: resume from adjudicate with the new judge — discovery
   is frozen and not re-run. Note: gpt-5.2 is the *calibration* judge (eval reproduction), not
   the product default — the product default is the Pareto-optimal pair from the judge-eval (§17).
   The validation we're about to do by hand in `bin/rejudge_hallucinations.py` becomes a
   first-class re-runnable primitive.
4. **Full re-run on a logic fix (no checkpoint)** → resume (§10.1): fixing the convergence
   predicate re-runs only the fix-loop node, with the union and discovery frozen from the prior
   run's audit log. The ~4M-token / ~20-min re-run we just suffered becomes a `--from verify`.
5. **The convergence bug itself (per-iteration vs cumulative)** → mock-AI testing (§18): the
   bug was deterministic logic, not an LLM failure. A scenario asserting "after iter 3 fixes 1
   bug but 7 cumulative remain, the loop does NOT declare ALL FIXED" would have caught it in CI
   in milliseconds, zero tokens, before any real run.

## 15. Open questions to resolve before build

1. **Language for built-in kinds:** the binary is Go. Built-in kinds (`review-lenses`,
   `match-then-adjudicate`, `still-present`) need Go implementations of the judge-call logic
   that currently lives in `harnesseval/judge.py` + `harnesseval/adjudicate.py`. Confirm Go is
   the implementation language (the binary is already Go); the Python logic is the spec to port.
2. **How the host agent discovers available FSM commands:** CLI tools an agent can call need
   to be surfaced. For Claude that's `--append-system-prompt` listing `mrv fsm *`; for Codex
   that's the shell. Confirm we ship a `mrv fsm --agent-prompt` that emits the tool-usage blurb.
3. **State storage location:** extend `.metareview/runs/<run-id>/` (existing) vs a new
   `.metareview/fsm/<run-id>/` namespace. Recommend: same namespace, new files (`state.json`,
   `audit.jsonl`) alongside existing review markdown.
4. **Token-budget accounting source:** for the `budget` convergence atom, where do token
   counts come from in an interactive session? The host agent doesn't always report per-call
   tokens to the binary. Options: (a) the judge calls report their own (covers the
   out-of-band cost, which is the controllable part), (b) the agent reports via
   `mrv fsm record tokens --data '{...}'`. Recommend (a) + (b) — judge calls self-report, agent
   records its own session totals best-effort.
5. **Idempotency contract per `kind` (from §10.1):** formalize which nodes are safely
   re-runnable from persisted inputs, which inputs must be frozen, which side effects (commits,
   work-tree files) must be reset on resume. The mock-AI test suite (§18) is what enforces this —
   a kind's resume test passing IS the idempotency contract being met. So this is less an open
   question and more a test to write per kind.
6. **Mock-AI scenario coverage (from §18):** which workflows get full scenario suites in 0.9.0?
   Minimum: the SDLC-loop workflow (the one we're building for). Each scenario is a checked-in
   YAML; coverage = every state transition, every gate pass/fail, every convergence atom,
   resume from each state, overflow. The real-LLM smoke test is the release gate, not CI.

## 16. Extension safety (escape-hatch guardrails)

The shell-command escape hatch (§6 user-defined `kind` via `cmd:`, §9 convergence `cmd` atom)
is the long-tail extension seam — and it executes arbitrary shell, so it is a security surface.
These guardrails are **non-negotiable in 0.9.0** — the hatch ships with them or it doesn't ship.

1. **Opt-in at init, never silent.** `fsm init` refuses a workflow containing any `cmd:` node or
   `cmd` convergence atom unless `--allow-custom-cmds` is passed. At init it prints every command
   that will run and requires confirmation. A workflow file dropped into a repo (PR, template,
   download) cannot auto-execute its commands without an explicit operator decision.
2. **Declare + verify.** The workflow YAML lists required `cmd:` paths; `fsm init` checks each
   exists and is executable, fails fast with `ERR_CMD_NOT_FOUND` before any run starts. No
   late-discovery "script missing" mid-loop.
3. **Timeout + schema validation.** Every `cmd` call has a configurable timeout (default 60s);
   stdout is validated against the declared output schema. Malformed output is a gate failure
   (`ERR_CMD_OUTPUT_INVALID`), not a silent pass or a crash. Non-zero exit is `ERR_CMD_FAILED`.
4. **Full audit.** Every `cmd` call is logged identically to a built-in: command string, input
   hash, output, exit code, duration, timestamp → `.metareview/runs/<run-id>/audit.jsonl`. Debugging
   a misbehaving custom atom uses the same audit log as a built-in.
5. **Documented as advanced, not default.** The docs lead with built-ins; the escape hatch is the
   power-user seam. **No released default workflow uses `cmd:` for normal loop logic** — the
   SDLC-loop and all shipped examples reference only the four built-in kinds + the four built-in
   convergence atoms for discover→adjudicate→fix→verify. The hatch exists for users who are not us
   (plug in their own adjudicator to validate the judge, call their CI as a gate).

   **One sanctioned exception — the overflow (safety-stop) handler.** A default workflow MAY
   declare an `on_overflow` `cmd:` that fires *only* when a built-in safety stop triggers
   (`max_iterations` or `budget`) — i.e. when the loop overflowed its bounds, not as part of
   normal logic. This is a fallback action, not a logic node: it cannot change state, only react.
   Use cases: dump final state for debugging, notify a channel, file a ticket, post to CI.
   The same guardrails (opt-in confirm, declare+verify, timeout+schema, full audit) apply; the
   handler runs with the final state snapshot on stdin and its output is audited, not consumed
   by the FSM. We don't *require* defaults to declare one, but if they do, this is the only
   sanctioned `cmd:` use in a shipped workflow.

**Why include it despite the surface:** the release's headline is swappable, auditable LLM calls,
and the most compelling swap (your own adjudicator, to validate gpt-5.2) is a shell command, not
a built-in. A closed box only swaps among four built-ins. The guardrails bound the risk; the
benefit is the extension story. **Why not use it in defaults:** defaults should be deterministic
across environments and versioned with the binary — a `cmd:`-dependent workflow isn't, so it's
an anti-pattern for shipped examples.

## 17. Judge model/effort selection (Pareto-optimal default, NOT gpt-5.2)

The product default `JUDGE` is **not** gpt-5.2. gpt-5.2 is the **calibration judge** — pinned to
reproduce Martian's published eval numbers (Phase A.1/A.3 calibration, `harnesseval/crosscheck.py`,
`harnesseval/inspect_a3.py`). Conflating the two would silently make every product user depend on a
model chosen for calibration parity, not quality or cost.

**The product default is determined by a separate judge-eval** (extending the Phase A.3 Inspect
harness, `harnesseval/run_a3.py` / `inspect_a3.py`, which already runs the Martian JUDGE_PROMPT via
Inspect's eval runner with native cost accounting). The judge-eval sweeps our available models ×
efforts and reports the Pareto frontier:

- **Quality axes:** recall, precision, AUC, R² (vs Martian's shipped decisions as ground truth,
  the same anchor A.3 uses).
- **Cost axes:** tokens (input + output, cache separately), wall time.
- **Selection rule:** pick the **knee** of the Pareto frontier — the model/effort where marginal
  quality gain per marginal token/time cost drops off. **Do NOT pick the absolute best/highest-
  effort model** if it ekes out a tiny AUC/R² increase for a large token/time cost. The whole point
  of the eval is the cost-quality tradeoff, not max quality at any cost.
- **Output:** a recommended `(JUDGE, JUDGE_EFFORT)` pair + the frontier table, committed to the
  repo as the documented 0.9.0 default. The binary then ships that as the `JUDGE` default (replacing
  `{required: true}` in §5 once the eval reports).

**Two config contexts (must stay separate):**
- **Product mode** (default): `JUDGE` = the Pareto-optimal pair from the judge-eval. This is what
  `mrv fsm init` uses for real workflows.
- **Calibration mode** (`mrv fsm init --calibration`): pins `JUDGE=gpt-5.2` so the eval's
  Phase-A reproduction stays bit-exact. Used only when re-running the eval, never in product.

Until the judge-eval reports, the binary ships **no hardcoded judge default** — `fsm init` requires
`--var JUDGE=<model>` and fails with `ERR_JUDGE_UNSET` rather than silently defaulting to gpt-5.2.
This forces the decision to be made deliberately, not inherited from the eval's calibration default.

**This also governs `JUDGE_EFFORT`** — same Pareto selection, not hardcoded `medium`.

## 18. Testing the FSM — TDD, 100% coverage, DI, mock AI factories

**The metareview Go binary already has 100% test coverage as a hard quality gate. 0.9.0 builds
on that standard, it does not lower it.** Every line of FSM logic lands under the same gate:
no PR merges if coverage drops, no feature ships without its tests. The FSM work is done **TDD**
(test-driven development) — the test for a transition/gate/predicate/resume path is written first,
watched fail for the right reason, then implemented to pass. The mock-AI factories below are what
make TDD possible for FSM logic that wraps nondeterministic LLM calls.

The FSM's value proposition is the **deterministic** part — the state machine, the gates, the
error codes, the convergence, the persistence, the resume. The nondeterministic part (LLM calls)
is by definition not unit-testable. **Mock the nondeterministic pieces, test the deterministic
skeleton exhaustively — end to end, behaviorally.** This is what makes the FSM trustworthy and
is a critical piece of 0.9.0 work — not a deferral.

### Dependency injection is the enabler

The FSM, the gates, the convergence predicates, and the run/audit log are pure Go types that
take their collaborators via interfaces — **dependency injection**. The `Kind` interface
(Appendix A), the `Predicate` interface (Appendix B), and a `Judge` interface are constructor-
injected, so a test wires the FSM with mock implementations and a real (temp-dir) run log — no
monkeypatching, no global state, no hidden network. This is the same DI discipline the existing
binary uses; 0.9.0 extends it to the new FSM types. DI is what makes the mock factories a clean
seam rather than a test-only hack: the production code path and the test path differ only in which
implementations are injected, nothing else.

```go
// The FSM depends on interfaces, not concrets. Tests inject mocks; prod injects reals.
type FSM struct {
    Kinds      map[string]Kind       // injected: real LLM kinds, or MockKind in tests
    Judge      Judge                 // injected: real cross-family HTTP judge, or MockJudge
    Store      RunStore              // injected: real JSONL run store, or in-memory for tests
    Predicates map[string]Predicate  // injected: real atoms, or scripted for tests
}
// Production wiring: FSM{Kinds: realKinds, Judge: realJudge, Store: jsonlStore, ...}
// Test wiring:      FSM{Kinds: mockKinds, Judge: mockJudge, Store: memStore,    ...}
```

### Mock AI factories

Every `kind` that makes an LLM call (`review-lenses`, `match-then-adjudicate`, `still-present`)
has a **mock implementation** registered alongside the real one, injected in tests via the DI
seam above (selected in prod via a `--mock-ai` flag or `MOCK_AI=1` env var for debugging). A mock
factory is deterministic and scriptable:

```go
// MockKind returns canned outputs for canned inputs, so the FSM logic is tested without
// any real LLM call. The factory reads a scenario file (input hash → output) and returns
// the scripted output; unscripted inputs are a test failure (not a silent pass).
type MockKind struct {
    Name      string
    Scenarios map[string]Output   // keyed by input hash
}
```

Scenario files are checked into the repo (e.g. `testdata/sdlc-loop/adjudicate.yaml`): each lists
input → scripted output. A test run loads the scenario, runs `fsm advance` with `--mock-ai`, and
asserts the FSM's state transitions, gate results, error codes, convergence decisions, and audit
log entries — **all deterministically, in milliseconds, with zero token cost.**

### What this lets us test exhaustively (the deterministic skeleton)

1. **State transitions** — every valid transition fires; every invalid transition is rejected
   with the right error (e.g. fix→done without `commit_exists` → `ERR_NO_COMMIT`).
2. **Gates** — each gate passes/fails on the scripted inputs and emits the exact error code.
   `commit_exists` on a clean tree + commit → pass; on uncommitted changes → `ERR_NO_COMMIT`
   with the diff; on no commit → `ERR_NO_COMMIT` with empty diff. All deterministic, all tested.
3. **Convergence predicates** — `all_fixed` stops on 0-unfixed; `no_fixation_progress` stops
   when unfixed-count plateaus; `budget` stops at the threshold; `any`/`all`/`not` compose
   correctly. The exact bug we just hit (per-iteration vs cumulative convergence) is a
   deterministic-logic bug — mock-AI testing would have caught it before any real run.
4. **Persistence + resume (§10.1)** — run to a checkpoint, kill, `advance --from <state>`, assert
   upstream nodes are NOT re-run (their audit entries are replayed, not regenerated) and the run
   continues correctly. This is the test that proves resume works and that idempotency holds.
5. **Audit log completeness** — every transition, gate, LLM call (mocked), and node output is
   logged with the right fields; two runs with different `JUDGE` vars produce diffable logs.
6. **Error propagation** — a mocked `ERR_NO_COMMIT` mid-run halts at `failed` with the right audit
   entry; resuming after the operator fixes the commit continues from verify.
7. **Overflow handler (§16)** — a mocked `max_iterations` trigger fires the `on_overflow` `cmd:`
   with the final state snapshot; assert it runs exactly once, only on overflow, not on normal
   convergence.

### Behavioral end-to-end integration tests

The unit tests above cover each piece. **Behavioral integration tests drive the whole FSM
end-to-end through a real workflow with every collaborator mocked** — the same DI wiring as
prod, only the kinds/judge/store are mocks. These are the tests that prove the pieces compose:

- **Happy path**: mock discover returns 3 findings, mock adjudicate confirms 2, the (mocked or
  real) agent-edit fixes both, mock verify reports 0 still-present → the FSM walks
  discover→adjudicate→fix→verify→done, asserts the full audit log + final state.
- **Convergence (cumulative)**: the exact regression we hit — mock discover returns bugs across
  3 iters such that iter 3 fixes its own 1 bug but 7 cumulative remain; assert the FSM does NOT
  emit "ALL FIXED" and does NOT transition to done; it loops until `no_fixation_progress` or
  `max_iterations`. This test would have caught the per-iteration-convergence bug in CI.
- **Gate failure + resume**: mock a fix node that leaves the tree dirty → `commit_exists` fails
  with `ERR_NO_COMMIT`; assert the run halts at `failed` with the right audit entry; "operator"
  (test) commits the changes; `advance --from verify` resumes and continues. Assert upstream
  nodes (discover/adjudicate) are replayed from the audit log, NOT re-executed.
- **JUDGE swap via resume**: run to done with `JUDGE=gpt-5.2`; resume `--from adjudicate` with
  `JUDGE=claude-opus-5`; assert discover is replayed (not re-run) and only adjudicate/verify
  re-execute; diff the two runs' audit logs. This is the §17 adjudicator-validation workflow,
  proven as a test.
- **Overflow**: mock `max_iterations` firing; assert `on_overflow` runs once with the final
  state snapshot, normal convergence paths never trigger it.

These are deterministic, run in CI in seconds, zero token cost, and are the release gate for
FSM correctness. The real-LLM smoke test below is a separate, manual gate for the
nondeterministic glue.

### Why this is critical, not optional

- **The binary already has 100% coverage; the FSM must not lower it.** TDD + the hard coverage
  gate are the existing quality standard. New FSM code that drops coverage below 100% does not
  merge.
- **The three bugs we hit this session were all deterministic-logic bugs**, not LLM bugs: the
  silent 0-fix (missing `commit_exists` gate), the per-iteration convergence (wrong predicate
  scope), the full-re-run on a logic fix (no resume). TDD + mock-AI behavioral tests catch
  exactly this class — before any real run, in CI, zero tokens.
- **LLM calls can't be CI-tested** — they cost money, are nondeterministic, and rate-limited.
  The deterministic skeleton is the part that MUST be in CI, and mock factories make it possible.
- **Resume (§10.1) is only trustworthy if tested** — without a mock-AI test that "run to N,
  kill, resume, assert no upstream re-run" passes, the idempotency contract is aspirational.
- **The adjudicator-validation workflow (§17) depends on resume** — re-running with
  `JUDGE=claude-opus-5` from adjudicate must not re-run discovery. A behavioral test proves it.

### Scope for 0.9.0

- **TDD for all FSM code** — tests written first, watched fail for the right reason, then
  implemented; 100% coverage maintained as the hard merge gate (existing binary standard)
- **DI wiring** — FSM/Kinds/Judge/Store/Predicates behind interfaces, constructor-injected; prod
  and test differ only in injected implementations
- Mock factory for each LLM `kind` (3 mocks: lenses, adjudicate, still-present) + a
  `--mock-ai`/`MOCK_AI` selector for prod debugging
- Scenario files for the SDLC-loop workflow covering: happy path (all fixed), convergence on
  no-progress, gate failures (ERR_NO_COMMIT, ERR_NO_FINDINGS), resume from each state, overflow
- **Behavioral end-to-end integration tests** driving the whole FSM with all collaborators
  mocked (happy path, cumulative-convergence regression, gate-failure+resume, JUDGE-swap-via-
  resume, overflow) — the release gate for FSM correctness
- A Go test suite asserting transitions/gates/convergence/persistence/resume/audit against the
  scenarios — runs in CI in seconds, zero tokens
- One "smoke" integration test with real LLM calls (not mocked) gated behind a `--smoke` flag
  and run manually before releases, not in CI (the eval itself is the real-world integration test)

---

## Appendix A: Concrete `kind` plugin interface sketch (for review)

```go
// Kind is a registered classifier node type. The FSM depends on this interface, not on any
// concrete implementation — this is the DI seam (§18) that lets tests inject MockKind while
// prod injects the real LLM-backed kinds. Production and test wiring differ only here.
type Kind interface {
    Name() string
    DefaultExec() ExecMode           // inline | subagent | fork
    IsLLM() bool                      // whether model/effort config applies
    InputSchema() *jsonschema.Schema
    OutputSchema() *jsonschema.Schema
    Run(ctx context.Context, in Input, cfg NodeConfig) (Output, error)
}

// Judge is the cross-family LLM-as-judge seam (§7, §17). Real impl does HTTP to a non-session
// model; MockJudge returns scripted verdicts. Same DI pattern — the FSM never calls a concrete.
type Judge interface {
    Call(ctx context.Context, kind string, in Input, cfg NodeConfig) (Verdict, error)
}

// RunStore is the persistence seam (§10). Real impl is JSONL on disk; tests use an in-memory
// store. The FSM never touches the filesystem directly — only through this interface.
type RunStore interface {
    Init(runID string) error
    AppendEvent(runID string, ev Event) error      // transitions, gates, LLM calls, node outputs
    LoadState(runID string) (State, error)          // for resume (§10.1)
    LoadEvents(runID string, from State) ([]Event, error)  // replay upstream on resume
}

// NodeConfig binds workflow vars to a node.
type NodeConfig struct {
    Model    string            // resolved from $VAR
    Effort   string
    Vars     map[string]string // workflow-level vars
    RunID    string            // for audit logging
}

// Registry — built-ins registered at init(), user kinds loaded from config.
var kinds = map[string]Kind{}
func Register(k Kind) { kinds[k.Name()] = k }
func Get(name string) (Kind, error) { ... }
```

User-defined kind (escape hatch, same pattern as convergence):
```yaml
nodes:
  adjudicate:
    kind: my-adjudicator       # not a built-in
    exec: fork
    cmd: "./my-adjudicate.sh"  # shell command: reads input JSON on stdin, writes output JSON
    schema: {input: ..., output: ...}
```

## Appendix B: Concrete convergence predicate interface sketch (for review)

```go
// Predicate is a composable convergence check.
type Predicate interface {
    Evaluate(state State) (stop bool, reason string, err error)
    Name() string
}

// Built-in atoms
type AllFixed struct{}
type NoFixationProgress struct{}
type Budget struct{ Tokens int }
type MaxIterations struct{ N int }

// Compose
type Any struct{ Of []Predicate }
type All struct{ Of []Predicate }
type Not struct{ P Predicate }

// Escape hatch
type CmdPredicate struct{ Cmd string; Args []string }
// runs the shell command with state JSON on stdin; reads {stop, reason} from stdout

// Parse from YAML:
//   any: [all_fixed, {budget: {tokens: 4000000}}]
//   → Any{Of: [AllFixed{}, Budget{Tokens: 4000000}]}
func ParsePredicate(yamlNode yaml.Node) (Predicate, error)
```

## Appendix C: Concrete state machine interface sketch (for review)

```go
type State string  // discover | adjudicate | fix | verify | done | failed

type Transition struct {
    From, To State
    Gate     string   // gate name to check
    Converge *Predicate // optional: checked at loop boundaries
    OnError  State     // default: failed
}

type FSM struct {
    RunID       string
    State       State
    Vars        map[string]string
    History     []TransitionEvent
    AuditLog    *AuditLogger
}

// Init creates a run, resolves vars, validates the workflow, returns initial state.
func Init(workflow Workflow, vars map[string]string) (*FSM, error)

// Advance attempts the next transition. Returns the gate result (pass/fail + error code)
// and, if a node must run, whether the agent should execute it (NEEDS_INPUT) or the binary
// ran it (e.g. a fork judge call). Deterministic gates always run first.
func (f *FSM) Advance() (AdvanceResult, error)

type AdvanceResult struct {
    NewState    State
    GateResult  GateResult  // {passed: bool, errorCode: string, detail: string}
    NodeWork    *NodeWork   // non-nil if the agent must do a node's work
    Converged   bool
    Reason      string
}
```
