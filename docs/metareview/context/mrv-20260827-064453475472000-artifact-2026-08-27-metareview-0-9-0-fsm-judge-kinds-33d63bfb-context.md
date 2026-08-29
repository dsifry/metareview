# metareview context: docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md

Run ID: `mrv-20260827-064453475472000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb`

## Target

- Path: `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `10980df`

## Artifact Excerpt

````markdown
# metareview 0.9.0 — spec 4: guardrails, judge, kinds, and mock AI

> **Status:** DRAFT r2 (2026-08-27). Fourth of the five split artifacts (ownership ledger: run spec §12). Owns plan
> r3 §1.8 items 2–5 (the runner-side guardrails; items 1 and consent are spec 2's `Init`), §2 (judge port), kinds/
> `Executor`/`Delta` producers, match-then-adjudicate composition + `Bug.Verdict` vocabulary, `index` assignment, the
> `llm_call`/`cmd_call` producer contract, mock scenarios, and the pinned harnesseval provenance of the prompts.
> Implements spec 2 r2's `machine.NodeKind`/`Executor`/`Registry`, `converge.Runner`, and `workflow.KindInfo`.
>
> **r2 changes** (review `mrv-20260827-062655870118000-…fsm-judge-kinds-33d63bfb`, 7 NEEDS_REVISION + Security PASS):
> calibration mode is bit-exact Python for all three kinds (two `still-present` templates); `match` parse errors skip
> the pair (Python parity); `RenderPrompt` defined as single-pass `str.format`; provenance test against the sibling
> repo + pinned python sha; `Request.Iter`; `llm_call` mapping table; `Guarded.Run` = `Call` guarantees, name-only;
> process-group kill; timer seam; redirects refused outright; redacted cmd payload; producer caps; effort mapping for
> every provider (or a hard error, never a silent no-op); glm/kimi floors; mock scenarios content-pinned.
>
> **Port spec:** `~/Developer/harnesseval/harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router}.py` @ `19ff9a8`
> (sibling repo, not vendored). Slot sources: `match` `golden_comment = Golden.Comment`, `candidate = Finding.IssueText`
> (`sdlc_loop.py:264` passes `f.issue_text` bare); `adjudicate` `candidate = Finding.IssueText`; `still-present`
> `golden_comment = Bug.Desc` (`sdlc_loop.py:353 _desc`). `match` runs at `max_tokens=1024` in the loop
> (`sdlc_loop.py:274`; 256 is the offline-eval default) — 1024 is parity.

---

## 1. Packages
```
internal/fsm/cmdexec   Runner (exec-backed + fake); Guarded (allow-list, pinned argv, hash re-verify, timeout, typed decode, audit)
internal/fsm/judge     Judge iface; prompts + fencing; parsers; providers; retry; tokens; MockJudge; NewHTTPClient
internal/fsm/kind      NodeKind/Executor implementations: review-lenses, match-then-adjudicate, still-present, agent-edit, cmd; Registry
internal/fsm/mockai    scenario files for MockJudge and the fake Runner; content hash
```
`errs`, `run` ← all. `converge` ← `cmdexec` (returns `converge.CmdResult`). `workflow`, `machine` (interfaces) ← `kind`.
`judge`, `cmdexec` ← `kind`. `judge`, `cmdexec` ← `mockai`. `machine` imports none of these.

## 2. `cmdexec`
```go
type Spec struct { Argv []string; Dir string; Stdin []byte; Timeout time.Duration; Env []string /* full environment, KEY=VALUE */ }
type Result struct { Stdout, Stderr []byte; ExitCode int; Duration time.Duration }
type Runner interface { Run(ctx, Spec) (Result, error) }
type Guarded struct { Runner Runner; Allowed []run.AllowedCmd; Dir string; RunID string; FileHash func(string) (string, error); Audit func(run.Event) error; Environ func() []string; Clock func() time.Time }
func (g Guarded) Run(ctx, name string, stdin []byte) (converge.CmdResult, error)   // the ONLY entry point (converge.Runner)
func (g Guarded) Call(ctx, name string, stdin []byte, out any) error               // Run + typed decode into out
```
`Run`: `name ∈ Allowed` (`ERR_CMD_NOT_ALLOWED{name}`); re-hash every `FileHashes` entry and re-scan argv per spec 2 §2.5
(`ERR_CMD_CHANGED`); execute **`Allowed[name].Argv` verbatim** (argv[0] is the pinned absolute path) in `Dir` with
`Timeout = TimeoutMS` (default 60 s, `ERR_CMD_TIMEOUT`); environment = `PATH`, `HOME`, `LANG`, `TMPDIR` from `Environ()` +
`MRV_RUN_ID=<RunID>` + each name in `Allowed[name].Env` that is set (declared in the workflow's `cmds.<name>.env`, part of
the consent sha); non-zero exit → `ERR_CMD_FAILED{exit}`; **every** run (including refusals after the allow-list check and
failures) appends `cmd_call{Name, Argv, Inpu
````

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
